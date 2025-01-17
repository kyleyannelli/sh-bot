package main

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
	"github.com/lmittmann/tint"
)

const version = "sh-bot v1.0.2"
const DEBOUNCE_PERIOD = 5 * time.Second

var (
	cooldownBtwnScripts = 30 * time.Second

	lastLoggedMsg string

	onlineScript  = flag.String("online-script", "", "REQUIRED: Location of the script to run when someone is online. Local or full path.")
	offlineScript = flag.String("offline-script", "", "REQUIRED: Location of the script to run when everyone goes offline. Local or full path.")
	startOnline   = flag.Bool("start-online", false, "OPTIONIAL: Sometimes presence states are unknown when the bot starts. If you want the bot to initialize by running the online script, choose this.")
	startOffline  = flag.Bool("start-offline", false, "OPTIONAL: Sometimes presence states are unknown when the bot starts. If you want the bot to initialize by running the offline script, choose this.")
	requireVc     = flag.Bool("vc-only", false, "OPTIONAL: Using this flag makes it so that the bot only fires scripts based on if any members are in the voice channel.")

	lastPresenceState      = make(map[string]discordgo.Status)
	lastPresenceStateMutex sync.RWMutex

	idsToTrack     = make(map[string]struct{})
	voiceChannelId string

	anyOnlineMutex            sync.RWMutex
	anyOnlineLastChange       time.Time
	anyOnline                 bool
	recievedAnyPresenceChange bool

	anyInVoiceChannelMutex sync.RWMutex
	anyInVoiceChannel      bool

	awaitingScriptFinishMutex sync.Mutex
	awaitingScriptFinish      bool

	lastScriptRun    time.Time
	onlineScriptRan  bool
	offlineScriptRan bool
)

func parseFlags() {
	flag.Parse()
	validateScript(*onlineScript)
	validateScript(*offlineScript)
	if *startOffline && *startOnline {
		panic("You cannot choose to start both online and offline scripts for the first run. Pick one.")
	}
}

func main() {
	parseFlags()

	if err := godotenv.Load(); err != nil {
		panic(err)
	}

	cooldownEnv := os.Getenv("COOLDOWN_BTWN_SCRIPTS_SECONDS")
	if cooldownInt, err := strconv.Atoi(cooldownEnv); err == nil {
		cooldownBtwnScripts = time.Duration(cooldownInt) * time.Second
	} else if cooldownEnv != "" {
		slog.Warn(fmt.Sprintf("Could not parse cooldown seconds %s. Ensure your value is only an int.", cooldownEnv))
	}

	voiceChannelIdEnv := os.Getenv("VOICE_CHANNEL")
	if _, err := strconv.Atoi(voiceChannelIdEnv); err == nil {
		voiceChannelId = voiceChannelIdEnv
	} else if voiceChannelIdEnv != "" {
		slog.Warn(fmt.Sprintf("Provided voice channel %v is not a valid channel id! Ignoring VC...", voiceChannelIdEnv))
	}
	if voiceChannelId == "" && *requireVc {
		panic("Required voice channel, but have an empty voice channel ID! Check your .env")
	}

	var botToken string
	if botToken = os.Getenv("DISCORD_BOT_TOKEN"); botToken == "" {
		panic("You cannot have an empty DISCORD_BOT_TOKEN in your .env!")
	}

	discord, err := discordgo.New("Bot " + botToken)
	if err != nil {
		panic(err)
	}

	discord.Identify.Intents |= discordgo.IntentsGuildPresences
	discord.Identify.Intents |= discordgo.IntentsGuildMembers

	if voiceChannelId != "" && *requireVc {
		discord.Identify.Intents |= discordgo.IntentsGuildVoiceStates
		discord.AddHandler(voiceChannelUpdate)
	} else if voiceChannelId != "" {
		discord.Identify.Intents |= discordgo.IntentsGuildVoiceStates
		discord.AddHandler(voiceChannelUpdate)
		discord.AddHandler(presenceUpdate)
	} else {
		discord.AddHandler(presenceUpdate)
	}

	if *startOnline {
		slog.Info("Running online script before starting the bot!")
		runOnlineScript()
	} else if *startOffline {
		slog.Info("Running offline script before starting the bot!")
		runOfflineScript()
	}
	err = discord.Open()
	defer discord.Close()
	if err != nil {
		panic(err)
	}

	setupRequired(discord)

	go dumbDetermine()

	catchSignals()
}

func areAnyOnline() bool {
	anyInVoiceChannelMutex.RLock()
	if *requireVc && anyInVoiceChannel {
		anyInVoiceChannelMutex.RUnlock()
		return true
	} else if *requireVc && !anyInVoiceChannel {
		anyInVoiceChannelMutex.RUnlock()
		return false
	} else if !*requireVc && anyInVoiceChannel {
		anyInVoiceChannelMutex.RUnlock()
		return true
	}
	anyInVoiceChannelMutex.RUnlock()

	lastPresenceStateMutex.RLock()
	defer lastPresenceStateMutex.RUnlock()
	for userId := range idsToTrack {
		state := lastPresenceState[userId]
		if state != "" && state != discordgo.StatusOffline {
			return true
		}
	}
	return false
}

func catchSignals() {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop
}

// A channel is definitely the more correct way to do this.
//
//	It just makes things a tiny more complex. Will switch over if the scope of this increases.
func dumbDetermine() {
	for {
		time.Sleep(500 * time.Millisecond)
		anyOnlineMutex.RLock()
		if !recievedAnyPresenceChange {
			anyOnlineMutex.RUnlock()
			logIfNotDuplicate("Haven't received any presence changes, not attempting to run a script until then.", slog.LevelDebug)
			continue
		}

		if time.Since(anyOnlineLastChange) < DEBOUNCE_PERIOD {
			anyOnlineMutex.RUnlock()
			logIfNotDuplicate("Debouncing...", slog.LevelDebug)
			continue
		}

		if (anyOnline && onlineScriptRan) || (!anyOnline && offlineScriptRan) {
			anyOnlineMutex.RUnlock()
			logIfNotDuplicate("Already ran script for current state.", slog.LevelDebug)
			continue
		}

		needToRelock := false
		if time.Since(lastScriptRun) < cooldownBtwnScripts {
			anyOnlineMutex.RUnlock()
			needToRelock = true
			sleepFor := cooldownBtwnScripts - time.Since(lastScriptRun)
			slog.Debug(fmt.Sprintf("Waiting %s to run script.", sleepFor))
			time.Sleep(sleepFor)
		}

		haventRanAnything := !onlineScriptRan && !offlineScriptRan

		if needToRelock {
			anyOnlineMutex.RLock()
		}

		if haventRanAnything && anyOnline {
			anyOnlineMutex.RUnlock()
			slog.Debug("Running online script for first run.")
			runOnlineScript()
			continue
		} else if haventRanAnything && !anyOnline {
			anyOnlineMutex.RUnlock()
			slog.Debug("Running offline script for first run.")
			runOfflineScript()
			continue
		}

		if anyOnline && !onlineScriptRan {
			anyOnlineMutex.RUnlock()
			slog.Debug("Running online script.")
			runOnlineScript()
		} else if !anyOnline && !offlineScriptRan {
			anyOnlineMutex.RUnlock()
			slog.Debug("Running offline script.")
			runOfflineScript()
		} else if needToRelock {
			anyOnlineMutex.RUnlock()
		}
	}
}

func loadIdsToTrack() {
	for _, id := range strings.Split(os.Getenv("USERS_IDS_TO_TRACK"), ",") {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		idsToTrack[id] = struct{}{}
	}
}

func presenceUpdate(discord *discordgo.Session, guildMember *discordgo.PresenceUpdate) {
	if _, exists := idsToTrack[guildMember.User.ID]; !exists {
		logIfNotDuplicate(fmt.Sprintf("Ignoring %s presence update", guildMember.User.ID), slog.LevelDebug)
		return
	}

	previousStatus := lastPresenceState[guildMember.User.ID]
	slog.Info(fmt.Sprintf("%s had previous status of %s and now has status of %s", guildMember.User.ID, previousStatus, guildMember.Status))
	lastPresenceStateMutex.Lock()
	lastPresenceState[guildMember.User.ID] = guildMember.Status
	lastPresenceStateMutex.Unlock()

	anyOnlineMutex.Lock()
	recievedAnyPresenceChange = true
	newOnlineState := areAnyOnline()
	if newOnlineState != anyOnline {
		anyOnline = newOnlineState
		anyOnlineLastChange = time.Now()
	}
	anyOnlineMutex.Unlock()
}

func runOfflineScript() {
	onlineScriptRan = false
	offlineScriptRan = true

	awaitingScriptFinishMutex.Lock()
	awaitingScriptFinish = true
	awaitingScriptFinishMutex.Unlock()

	runScript(*offlineScript)

	awaitingScriptFinishMutex.Lock()
	awaitingScriptFinish = false
	awaitingScriptFinishMutex.Unlock()
	lastScriptRun = time.Now()
}

func runOnlineScript() {
	onlineScriptRan = true
	offlineScriptRan = false

	awaitingScriptFinishMutex.Lock()
	awaitingScriptFinish = true
	awaitingScriptFinishMutex.Unlock()

	runScript(*onlineScript)

	awaitingScriptFinishMutex.Lock()
	awaitingScriptFinish = false
	awaitingScriptFinishMutex.Unlock()
	lastScriptRun = time.Now()
}

func runScript(script string) {
	cmd := exec.Command(script)
	strBld := new(strings.Builder)
	cmd.Stdout = strBld
	err := cmd.Run()

	print(strBld.String())

	if err != nil {
		slog.Warn(fmt.Sprintf("Error running script %s: %v", script, err))
	}
}

func setupLogs() *slog.Logger {
	w := os.Stdout

	logger := slog.New(
		tint.NewHandler(w, &tint.Options{
			AddSource:  true,
			TimeFormat: time.ANSIC,
			Level:      slog.LevelDebug,
		}),
	).With("version", version)

	slog.SetDefault(logger)

	return logger
}

func setupRequired(discord *discordgo.Session) {
	setupLogs()
	slog.Info("Loading IDs to track...")
	loadIdsToTrack()
	slog.Info("Grabbing existing user presence statuses...")
	stowStatuses(discord)
	slog.Info("sh-bot started. exit with crtl+c.")
}

func stowStatuses(discord *discordgo.Session) {
	for _, guild := range discord.State.Guilds {
		members, err := discord.GuildMembers(guild.ID, "", 1000)
		if err != nil {
			slog.Warn(fmt.Sprintf("Failed to get members for guild %s: %v", guild.ID, err))
			continue
		}

		if !*requireVc {
			for _, member := range members {
				if _, exists := idsToTrack[member.User.ID]; !exists {
					continue
				}
				if _, exists := lastPresenceState[member.User.ID]; exists {
					continue
				}

				status := discordgo.StatusOffline

				presence, err := discord.State.Presence(guild.ID, member.User.ID)
				if err != nil {
					slog.Warn(fmt.Sprintf("Failed to get presence for member %s in guild %s: %v", member.User.ID, guild.ID, err))
					continue
				}

				status = presence.Status
				lastPresenceState[member.User.ID] = status
				runAnyOnlineCheck()
			}
		}

		if voiceChannelId != "" {
			haveAny := false
			for _, member := range getMembersInVoiceChannel(discord, guild.ID, voiceChannelId) {
				if _, exists := idsToTrack[member.User.ID]; exists {
					haveAny = true
					break
				}
			}
			markVoiceChannelPresenceChange(haveAny)
		}
	}
}

func validateScript(script string) {
	if script == "" {
		panic("Please provide a path for the script to run!")
	}

	if _, err := os.Stat(script); errors.Is(err, os.ErrNotExist) {
		panic(fmt.Sprintf("Couldn't find file %s!", script))
	}

	fileInfo, err := os.Stat(script)
	if err != nil {
		panic(err)
	} else if fileInfo.Mode()&0111 == 0 {
		panic("File is not executable!")
	}
}

func voiceChannelUpdate(discord *discordgo.Session, vs *discordgo.VoiceStateUpdate) {
	updateVoiceChannelId := ""

	// The following 2 if statements aren't needed, but makes the flow more understandable.
	haveCurrentChannelId := vs.ChannelID != ""
	if haveCurrentChannelId {
		updateVoiceChannelId = vs.ChannelID
	}

	havePreviousChannelId := vs.BeforeUpdate != nil && vs.BeforeUpdate.ChannelID != ""
	if !haveCurrentChannelId && havePreviousChannelId {
		updateVoiceChannelId = vs.BeforeUpdate.ChannelID
	}

	badVoiceChannelId := voiceChannelId != updateVoiceChannelId
	if badVoiceChannelId {
		logIfNotDuplicate(fmt.Sprintf("Ignoring channel update for channel %v", vs.ChannelID), slog.LevelDebug)
		return
	}

	members := getMembersInVoiceChannel(discord, vs.GuildID, updateVoiceChannelId)

	haveAny := false
	for _, member := range members {
		if _, exists := idsToTrack[member.User.ID]; exists {
			haveAny = true
			break
		}
	}

	markVoiceChannelPresenceChange(haveAny)
}

func markVoiceChannelPresenceChange(haveAnyMembers bool) {
	anyInVoiceChannelMutex.Lock()
	anyInVoiceChannel = haveAnyMembers
	anyInVoiceChannelMutex.Unlock()

	runAnyOnlineCheck()
}

func runAnyOnlineCheck() {
	anyOnlineMutex.Lock()
	recievedAnyPresenceChange = true
	newOnlineState := areAnyOnline()
	if newOnlineState != anyOnline {
		anyOnline = newOnlineState
		anyOnlineLastChange = time.Now()
	}
	anyOnlineMutex.Unlock()
}

func getMembersInVoiceChannel(discord *discordgo.Session, guildID, channelID string) []*discordgo.Member {
	var membersInChannel []*discordgo.Member

	guild, err := discord.State.Guild(guildID)
	if err != nil {
		slog.Warn(fmt.Sprintf("Error getting guild state: %v", err))
		return nil
	}

	for _, voiceState := range guild.VoiceStates {
		if voiceState.ChannelID == channelID {
			member, err := discord.State.Member(guildID, voiceState.UserID)

			if err == nil {
				membersInChannel = append(membersInChannel, member)
			} else {
				slog.Warn(fmt.Sprintf("Error retrieving member information: %v", err))
			}
		}
	}

	return membersInChannel
}

func logIfNotDuplicate(msg string, level slog.Level) {
	if msg == lastLoggedMsg {
		return
	}
	lastLoggedMsg = msg

	switch level {
	case slog.LevelInfo:
		slog.Info(msg)
	case slog.LevelWarn:
		slog.Warn(msg)
	case slog.LevelError:
		slog.Error(msg)
	case slog.LevelDebug:
		slog.Debug(msg)
	}
}

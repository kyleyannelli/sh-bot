# sh-bot

sh-bot is a [Go-based](https://github.com/bwmarrin/discordgo) Discord bot that automates can automate things like your game server's lifecycle, ensuring it’s online only when needed based on who’s in your Discord server. Whether you're managing a Factorio server with friends or any other game, sh-bot runs scripts to start or stop your server when specified users go online or join voice channels, helping to save system resources when no one’s playing.

Imagine having your game server fire up automatically when your friends are online and shut down when the last one logs off—all without manual intervention. That’s exactly what sh-bot does.

# Table of Contents

1. [Introduction](#sh-bot)
2. [Overview](#overview)
3. [Features](#features)
   - [Project Overview](#project-overview)
4. [Command-Line Flags](#command-line-flags)
   - [Example Command](#example-command)
5. [Environment Configuration (`.env` File)](#environment-configuration-env-file)
6. [Security Considerations](#security-considerations)
7. [Example Shell Scripts](#example-shell-scripts)
   - [`start.sh`](#startsh)
   - [`stop.sh`](#stopsh)
8. [Launching the Bot](#launching-the-bot)
9. [Debugging Tips](#debugging-tips)
10. [Disclaimer](#disclaimer)

## Overview

This bot monitors the online/offline status and voice channel activity of specified Discord users. When any tracked user comes online or joins a designated voice channel, the bot executes a predefined "online" script (e.g., to start a game server). Conversely, when all tracked users go offline or leave the voice channel, it runs an "offline" script (e.g., to stop the game server). This helps in conserving resources by ensuring the game server is only running when needed.

## Features

### **Project Overview**
This Go-based bot allows you to automate shell script executions based on user presence in a Discord server. The bot is particularly useful for scenarios like launching or shutting down a game server (e.g., a Factorio server) depending on whether your friends are online and in a voice channel.

**Key Features:**
- **User Presence Monitoring**: Tracks the online/offline status of specified users.
- **Voice Channel Monitoring**: Optionally monitors a specific voice channel for user activity.
- **Automated Script Execution**: Runs custom scripts based on user presence or voice channel activity.
- **Resource Management**: Helps in freeing up system resources by stopping services when not in use.
- **Configurable Cooldown**: Prevents rapid toggling of scripts through a customizable cooldown period.
- **Debouncing Logic**: Ensures scripts are not executed multiple times due to quick presence fluctuations.

---

### **Command-Line Flags**

The bot uses several command-line flags for configuration:

| **Flag**           | **Type** | **Description**                                                                                                                                                          | **Required** | **Usage**                   |
|--------------------|----------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------|--------------|-------------------------------|
| `-online-script`   | String   | **REQUIRED**. Path to the script that runs when someone comes online. Local or full path is acceptable.                                                                 | Yes          | `-online-script "/opt/server/start.sh"`   |
| `-offline-script`  | String   | **REQUIRED**. Path to the script that runs when everyone goes offline. Local or full path.                                                                              | Yes          | `-offline-script "/opt/server/stop.sh"`   |
| `-start-online`    | No Input | **OPTIONAL**. Initialize the bot in the "online" state, running the online script at startup. Cannot be used with `-start-offline`.                                      | No           | `-start-online`               |
| `-start-offline`   | No Input | **OPTIONAL**. Initialize the bot in the "offline" state, running the offline script at startup. Cannot be used with `-start-online`.                                     | No           | `-start-offline`              |
| `-vc-only`         | No Input | **OPTIONAL**. If enabled, the bot triggers scripts based on whether members are present in a specific voice channel.                                                     | No           | `-vc-only`                    |

### **Example Command**
```bash
./sh-bot -online-script "./start.sh" -offline-script "./stop.sh" -start-online -vc-only
```

---

### **Environment Configuration (`.env` File)**

To configure the bot, create a `.env` file with the following values:

```dotfile
# REQUIRED
DISCORD_BOT_TOKEN="YOUR_DISCORD_BOT_TOKEN"
USERS_IDS_TO_TRACK="1482588888810182737, 1192874312810182737, 1234568888810182737"

# OPTIONAL
COOLDOWN_BTWN_SCRIPTS_SECONDS="30"
VOICE_CHANNEL="1921714129813191711"
```

- **`DISCORD_BOT_TOKEN`**: Token obtained from the Discord Developer Portal.
- **`USERS_IDS_TO_TRACK`**: Comma-separated list of Discord user IDs that the bot tracks.
- **`COOLDOWN_BTWN_SCRIPTS_SECONDS`**: The cooldown period between running scripts (default: 30 seconds).
- **`VOICE_CHANNEL`**: The specific voice channel ID the bot monitors, only required if using `-vc-only`.

---

### **Security Considerations**

**⚠️ WARNING: This bot executes shell scripts based on Discord events. Improper use can lead to security risks. Exercise caution! ⚠️**

- **Avoid Running Privileged Commands**: Do not use this bot to run scripts requiring sudo privileges. Create a user for the bot that can run specific commands.

---

### **Example Shell Scripts**

I am using `sh-bot` to start and stop a game server, you might have scripts like these:

#### **`start.sh`**
```bash
#!/bin/bash
echo "Starting Factorio server..."
systemctl start factorio
echo "Factorio server started..."
```

#### **`stop.sh`**
```bash
#!/bin/bash
echo "Stopping Factorio server..."
systemctl stop factorio
echo "Factorio server stopped..."
```

Make sure these scripts are **executable**:
```bash
chmod +x start.sh stop.sh
```

---

### **Launching the Bot**

1. Create and populate the `.env` file. I've provided `.env.example` for you. See 
2. Write the `online` and `offline` scripts (e.g., `start.sh`, `stop.sh`).
3. Ensure the scripts are executable.
4. Use the appropriate flags to run the bot. In this example I would like to start and stop my game server, only on the basis of the voice channel presence:
   ```bash
   ./sh-bot -online-script "./start.sh" -offline-script "./stop.sh" -start-online -vc-only
   ```

---

### **Debugging Tips**
- **Check Log Outputs**: Use the log outputs to verify the bot's behavior. It will log presence updates, script executions, and any errors.
- **Debounce Period**: The `DEBOUNCE_PERIOD` is set to 5 seconds to avoid rapid, consecutive script triggers. If you are receiving many events that change the overall presence state, nothing will trigger until the state stabilizes.
- **Cooldown Between Scripts**: `COOLDOWN_BTWN_SCRIPTS_SECONDS` defaults to 30 seconds to avoid rapid execution of your scripts. This can be set to 0!

--- 

### **Disclaimer**

This bot is designed for lightweight, non-critical automation tasks (e.g., launching game servers). It **should not be used** for sensitive, security-critical operations without thorough testing and security assessments.

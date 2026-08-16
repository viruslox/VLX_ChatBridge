package bot

// This file adds Discord slash-command support to the bot, independently of the
// legacy prefix-message path in discord.go. It provides:
//
//   /commands (alias /comandi) : owner-only, ephemeral list of available
//       commands, grouped into native Discord commands and reserved
//       Twitch/YouTube chat commands (with optional descriptions).
//   /run <command>             : owner-only, executes a reserved multi_action
//       (JSON) command by emitting its ipc_control actions on the global
//       control channel; confirms ephemerally on Discord.
//
// The bot scans the command folder itself (via twitch.ScanAudioCommands), so it
// stays decoupled from the ChatFlow module. Execution is intentionally limited
// to multi_action (JSON) commands.

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/snowflake/v2"

	coreevents "VLX_ChatBridge/internal/core/events"
	chattwitch "VLX_ChatBridge/internal/modules/chatflow/twitch"
)

// slashCommandDefs are the guild slash-commands this bot registers.
func slashCommandDefs() []discord.ApplicationCommandCreate {
	return []discord.ApplicationCommandCreate{
		discord.SlashCommandCreate{
			Name:        "commands",
			Description: "List available owner commands",
		},
		discord.SlashCommandCreate{
			Name:        "comandi",
			Description: "Elenca i comandi disponibili per l'owner",
		},
		discord.SlashCommandCreate{
			Name:        "run",
			Description: "Run a reserved Twitch/YouTube multi-action command",
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionString{
					Name:        "command",
					Description: "The reserved command name to run",
					Required:    true,
				},
			},
		},
		discord.SlashCommandCreate{
			Name:        "join",
			Description: "Bot joins a voice channel and starts the SRT stream",
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionChannel{
					Name:        "channel",
					Description: "Voice channel to join (defaults to your current channel)",
					Required:    false,
				},
			},
		},
		discord.SlashCommandCreate{
			Name:        "leave",
			Description: "Bot stops streaming and disconnects from voice",
		},
		discord.SlashCommandCreate{
			Name:        "reload",
			Description: "Reload the AudioBridge module",
		},
		discord.SlashCommandCreate{
			Name:        "shutdown",
			Description: "Shut down VLX_ChatBridge",
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionBool{
					Name:        "confirm",
					Description: "Set to true to confirm shutdown",
					Required:    true,
				},
			},
		},
	}
}

// registerSlashHandlers wires the slash-command router and returns it so the
// caller can attach it as an event listener. It does not perform I/O.
func (b *DiscordBot) registerSlashHandlers() *handler.Mux {
	r := handler.New()
	r.SlashCommand("/commands", b.handleSlashCommands)
	r.SlashCommand("/comandi", b.handleSlashCommands)
	r.SlashCommand("/run", b.handleSlashRun)
	r.SlashCommand("/join", b.handleSlashJoin)
	r.SlashCommand("/leave", b.handleSlashLeave)
	r.SlashCommand("/reload", b.handleSlashReload)
	r.SlashCommand("/shutdown", b.handleSlashShutdown)
	return r
}

// isInteractionOwner reports whether the interaction was invoked by the guild owner.
func (b *DiscordBot) isInteractionOwner(e *handler.CommandEvent) bool {
	if e.GuildID() == nil {
		return false
	}
	guild, ok := e.Client().Caches.Guild(*e.GuildID())
	if !ok {
		return false
	}
	return guild.OwnerID == e.User().ID
}

// ephemeral builds an ephemeral message create with the given content.
func ephemeral(content string) discord.MessageCreate {
	return discord.MessageCreate{
		Content: content,
		Flags:   discord.MessageFlagEphemeral,
	}
}

// handleSlashCommands responds with the grouped, ephemeral command list.
func (b *DiscordBot) handleSlashCommands(_ discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	if !b.isInteractionOwner(e) {
		return e.CreateMessage(ephemeral("Only the server owner can use this command."))
	}
	return e.CreateMessage(ephemeral(b.buildCommandList()))
}

// handleSlashRun executes a reserved multi_action command by name.
func (b *DiscordBot) handleSlashRun(data discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	if !b.isInteractionOwner(e) {
		return e.CreateMessage(ephemeral("Only the server owner can use this command."))
	}

	name := strings.ToLower(strings.TrimSpace(data.String("command")))
	if name == "" {
		return e.CreateMessage(ephemeral("Please provide a command name."))
	}

	cmds := b.scanCommands()
	cmd, ok := cmds[name]
	if !ok {
		return e.CreateMessage(ephemeral(fmt.Sprintf("Unknown command: %q", name)))
	}
	if cmd.MediaType != "multi_action" {
		return e.CreateMessage(ephemeral(fmt.Sprintf("Command %q is not runnable from Discord (only multi-action/JSON commands are).", name)))
	}

	emitted := b.emitMultiAction(name, cmd)
	if emitted == 0 {
		return e.CreateMessage(ephemeral(fmt.Sprintf("Command %q had no runnable ipc_control actions.", name)))
	}
	return e.CreateMessage(ephemeral(fmt.Sprintf("Executed %q (%d action(s) dispatched).", name, emitted)))
}

// emitMultiAction dispatches the ipc_control actions of a multi_action command
// onto the global control channel. Returns how many actions were dispatched.
func (b *DiscordBot) emitMultiAction(name string, cmd chattwitch.CommandData) int {
	emitted := 0
	for _, action := range cmd.Actions {
		actionType, _ := action["type"].(string)
		if actionType != "ipc_control" {
			continue
		}
		// Annotate like the chat path does, so downstream consumers match.
		action["is_broadcaster"] = true
		action["command"] = "!" + name

		outData, err := json.Marshal(action)
		if err != nil {
			continue
		}
		select {
		case coreevents.ControlBroadcastChan <- outData:
			emitted++
		default:
			// Channel full; skip rather than block the interaction.
		}
	}
	return emitted
}

// scanCommands scans the chat command folder autonomously.
func (b *DiscordBot) scanCommands() chattwitch.AudioCommandsMap {
	baseDir := filepath.Join(b.chatBridgeDIR, "static", "chat")
	cmds, err := chattwitch.ScanAudioCommands(baseDir, b.zapLogger)
	if err != nil || cmds == nil {
		return chattwitch.AudioCommandsMap{}
	}
	return cmds
}

// buildCommandList renders the grouped, human-readable command list.
func (b *DiscordBot) buildCommandList() string {
	var sb strings.Builder

	sb.WriteString("**Discord commands**\n")
	sb.WriteString("`/join [channel]` — bot joins a voice channel and starts the SRT stream\n")
	sb.WriteString("`/leave` — bot stops streaming and disconnects\n")
	sb.WriteString("`/reload` — reload the AudioBridge module\n")
	sb.WriteString("`/shutdown confirm:true` — shut down VLX_ChatBridge\n")
	sb.WriteString("`/run <command>` — run a reserved multi-action command\n")

	cmds := b.scanCommands()

	// Reserved (owner-only) commands, sorted for stable output.
	var reserved []string
	for name, data := range cmds {
		if data.IsBroadcasterOnly {
			reserved = append(reserved, name)
		}
	}
	sort.Strings(reserved)

	sb.WriteString("\n**Reserved Twitch/YouTube chat commands**\n")
	if len(reserved) == 0 {
		sb.WriteString("_(none found)_\n")
		return sb.String()
	}

	for _, name := range reserved {
		data := cmds[name]
		runnable := ""
		if data.MediaType == "multi_action" {
			runnable = " *(runnable via /run)*"
		}
		if strings.TrimSpace(data.Description) != "" {
			sb.WriteString(fmt.Sprintf("`!%s`%s — %s\n", name, runnable, data.Description))
		} else {
			sb.WriteString(fmt.Sprintf("`!%s`%s\n", name, runnable))
		}
	}

	return sb.String()
}

// syncSlashCommands registers the guild slash-commands with Discord. Requires an
// open gateway and a non-empty guild ID.
func (b *DiscordBot) syncSlashCommands() error {
	if b.guildID == "" {
		return fmt.Errorf("guild_id is empty; slash commands cannot be registered")
	}
	gid, err := snowflake.Parse(b.guildID)
	if err != nil {
		return fmt.Errorf("invalid guild_id %q: %w", b.guildID, err)
	}
	return handler.SyncCommands(b.client, slashCommandDefs(), []snowflake.ID{gid})
}

// handleSlashJoin joins a voice channel (option, else the caller's current one)
// and wires the SRT receiver/sender, mirroring the former prefix "join" path.
func (b *DiscordBot) handleSlashJoin(data discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	if !b.isInteractionOwner(e) {
		return e.CreateMessage(ephemeral("Only the server owner can use this command."))
	}
	if e.GuildID() == nil {
		return e.CreateMessage(ephemeral("This command must be used in a server."))
	}
	guildID := *e.GuildID()

	var channelID snowflake.ID
	if ch, ok := data.OptChannel("channel"); ok {
		channelID = ch.ID
	} else {
		vs, ok := e.Client().Caches.VoiceState(guildID, e.User().ID)
		if !ok || vs.ChannelID == nil {
			return e.CreateMessage(ephemeral("You are not in a voice channel, and no channel was provided."))
		}
		channelID = *vs.ChannelID
	}

	conn := b.client.VoiceManager.CreateConn(guildID)
	go func() {
		if err := conn.Open(context.TODO(), channelID, false, false); err != nil {
			log.Printf("[AudioBridge] Failed to join voice channel %s: %v", channelID, err)
			return
		}
		conn.SetOpusFrameReceiver(NewDiscordOpusReceiver(b.discordStreamingEnabled, b.excludedUsers))
		conn.SetOpusFrameProvider(NewDiscordPCMSender(b.discordOutChan))
		log.Printf("[AudioBridge] Joined voice channel %s successfully.", channelID)
	}()

	return e.CreateMessage(ephemeral(fmt.Sprintf("Joining <#%s> and starting the stream.", channelID)))
}

// handleSlashLeave disconnects from voice, mirroring the former prefix "leave" path.
func (b *DiscordBot) handleSlashLeave(_ discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	if !b.isInteractionOwner(e) {
		return e.CreateMessage(ephemeral("Only the server owner can use this command."))
	}
	if e.GuildID() == nil {
		return e.CreateMessage(ephemeral("This command must be used in a server."))
	}
	guildID := *e.GuildID()

	conn := b.client.VoiceManager.GetConn(guildID)
	if conn == nil {
		return e.CreateMessage(ephemeral("Not currently connected to a voice channel."))
	}
	go func() {
		conn.Close(context.TODO())
		b.client.VoiceManager.RemoveConn(guildID)
		log.Printf("[AudioBridge] Left voice channel successfully.")
	}()

	return e.CreateMessage(ephemeral("Leaving the voice channel."))
}

// handleSlashReload restarts the AudioBridge module, mirroring the former prefix "reload" path.
func (b *DiscordBot) handleSlashReload(_ discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	if !b.isInteractionOwner(e) {
		return e.CreateMessage(ephemeral("Only the server owner can use this command."))
	}
	log.Printf("[AudioBridge] Reloading AudioBridge module...")
	go func() {
		if b.controller != nil {
			b.controller.StopModule("AudioBridge")
			b.controller.StartModule("AudioBridge")
		}
	}()
	return e.CreateMessage(ephemeral("Reloading the AudioBridge module."))
}

// handleSlashShutdown terminates the process when confirm:true, replacing the
// former prefix "shutdown" + "yes" two-step confirmation.
func (b *DiscordBot) handleSlashShutdown(data discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	if !b.isInteractionOwner(e) {
		return e.CreateMessage(ephemeral("Only the server owner can use this command."))
	}
	if !data.Bool("confirm") {
		return e.CreateMessage(ephemeral("Shutdown not confirmed. Re-run with `confirm:true` to proceed."))
	}
	log.Printf("[AudioBridge] Shutdown confirmed by owner via slash command.")
	if err := e.CreateMessage(ephemeral("Shutting down...")); err != nil {
		log.Printf("[AudioBridge] Failed to acknowledge shutdown: %v", err)
	}
	go func() {
		syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
	}()
	return nil
}
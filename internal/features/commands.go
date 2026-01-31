package commands

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	dashboard "github.com/hxnx/tunebot/internal/features/dashboard"
	dashboardcmd "github.com/hxnx/tunebot/internal/features/dashboard/commands"
	dashboardlisteners "github.com/hxnx/tunebot/internal/features/dashboard/listeners"
	"github.com/hxnx/tunebot/internal/features/modals"
	musiccmd "github.com/hxnx/tunebot/internal/features/music/commands"
	musiclisteners "github.com/hxnx/tunebot/internal/features/music/listeners"
	queueview "github.com/hxnx/tunebot/internal/features/music/queueview"
	pingcmd "github.com/hxnx/tunebot/internal/features/ping/commands"
	pinglisteners "github.com/hxnx/tunebot/internal/features/ping/listeners"
	shared "github.com/hxnx/tunebot/internal/features/shared"
	"github.com/hxnx/tunebot/internal/music"
)

const musicQueueDefaultLimit = int64(10)

var (
	CommandList = []*discordgo.ApplicationCommand{
		{
			Name:        "핑",
			Description: "봇 상태를 확인합니다",
		},
		{
			Name:        "노래",
			Description: "노래 재생/관리 명령어",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "재생",
					Description: "노래를 검색해 재생합니다",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "정지",
					Description: "재생을 중지하고 대기열을 비웁니다",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "스킵",
					Description: "현재 곡을 건너뜁니다",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "대기열",
					Description: "현재 대기열을 표시합니다",
					Options: []*discordgo.ApplicationCommandOption{
						{
							Type:        discordgo.ApplicationCommandOptionInteger,
							Name:        "limit",
							Description: "표시할 곡 수",
							Required:    false,
						},
					},
				},

				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "반복",
					Description: "반복 모드를 설정합니다",
					Options: []*discordgo.ApplicationCommandOption{
						{
							Type:        discordgo.ApplicationCommandOptionString,
							Name:        "모드",
							Description: "꺼짐/곡/대기열",
							Required:    true,
							Choices: []*discordgo.ApplicationCommandOptionChoice{
								{
									Name:  "꺼짐",
									Value: "off",
								},
								{
									Name:  "곡 반복",
									Value: "track",
								},
								{
									Name:  "대기열 반복",
									Value: "queue",
								},
							},
						},
					},
				},
			},
		},
		{
			Name:        "대시보드",
			Description: "TuneBot 대시보드를 설정합니다",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:         discordgo.ApplicationCommandOptionChannel,
					Name:         "category",
					Description:  "대시보드 채널을 생성할 카테고리",
					Required:     false,
					ChannelTypes: []discordgo.ChannelType{discordgo.ChannelTypeGuildCategory},
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "channel_name",
					Description: "대시보드 채널 이름 (기본: 🎵-tunebot)",
					Required:    false,
				},
			},
		},
	}
	commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"핑":    pingcmd.Ping,
		"노래":   handleMusicGroupCommand,
		"대시보드": dashboardcmd.SetupDashboard,
	}
)

func handleMusicGroupCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}

	data := i.ApplicationCommandData()
	sub := getSubcommandOption(data)
	if sub == nil {
		shared.RespondEphemeral(s, i, "사용할 명령을 선택해 주세요.")
		return
	}

	switch sub.Name {
	case "재생":
		musiccmd.Play(s, i)
	case "정지":
		musiccmd.Stop(s, i)
	case "스킵":
		musiccmd.Skip(s, i)
	case "대기열":
		handleMusicQueueSubcommand(s, i, sub.Options)
	case "반복":
		handleMusicRepeatSubcommand(s, i, sub.Options)
	default:
		shared.RespondEphemeral(s, i, "지원하지 않는 노래 명령입니다.")
	}
}

func handleMusicQueueSubcommand(s *discordgo.Session, i *discordgo.InteractionCreate, options []*discordgo.ApplicationCommandInteractionDataOption) {
	if i.GuildID == "" {
		shared.RespondEphemeral(s, i, "이 명령어는 서버에서만 사용할 수 있습니다.")
		return
	}

	limit := shared.GetOptionInt64(options, "limit")
	if limit <= 0 {
		limit = musicQueueDefaultLimit
	}

	store := music.NewQueueStoreFromDefault()
	if store == nil {
		shared.RespondEphemeral(s, i, "대기열을 조회할 수 없습니다.")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	items, err := store.List(ctx, i.GuildID, 0)
	if err != nil {
		log.Printf("queue error: %v", err)
		shared.RespondEphemeral(s, i, "대기열을 불러오지 못했습니다.")
		return
	}

	if len(items) == 0 {
		shared.RespondEphemeral(s, i, "대기열이 비어 있습니다.")
		return
	}

	perPage := int(limit)
	components, _ := queueview.BuildQueueComponents(items, 1, perPage)

	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Components: components,
			Flags:      discordgo.MessageFlagsIsComponentsV2 | discordgo.MessageFlagsEphemeral,
		},
	}); err != nil {
		log.Printf("queue respond failed: %v", err)
	}
}

func handleMusicRepeatSubcommand(s *discordgo.Session, i *discordgo.InteractionCreate, options []*discordgo.ApplicationCommandInteractionDataOption) {
	if i.GuildID == "" {
		shared.RespondEphemeral(s, i, "이 명령어는 서버에서만 사용할 수 있습니다.")
		return
	}

	mode := strings.TrimSpace(shared.GetOptionString(options, "모드"))
	if mode == "" {
		shared.RespondEphemeral(s, i, "반복 모드를 선택해 주세요.")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	store := music.NewQueueStoreFromDefault()
	if store == nil {
		shared.RespondEphemeral(s, i, "설정을 저장할 수 없습니다.")
		return
	}

	settings, err := store.GetSettings(ctx, i.GuildID)
	if err != nil {
		shared.RespondEphemeral(s, i, "현재 설정을 불러오지 못했습니다.")
		return
	}

	var label string
	switch mode {
	case "off":
		settings.RepeatMode = music.RepeatModeNone
		label = "꺼짐"
	case "track":
		settings.RepeatMode = music.RepeatModeTrack
		label = "곡 반복"
	case "queue":
		settings.RepeatMode = music.RepeatModeQueue
		label = "대기열 반복"
	default:
		shared.RespondEphemeral(s, i, "지원하지 않는 반복 모드입니다.")
		return
	}

	if err := store.SetSettings(ctx, i.GuildID, settings); err != nil {
		shared.RespondEphemeral(s, i, "설정 저장에 실패했습니다.")
		return
	}

	dashboard.UpdateDashboardSettingsCache(i.GuildID, settings)

	if err := dashboard.UpdateDashboardByGuild(s, i.GuildID); err != nil {
		log.Printf("failed to update dashboard after repeat set: %v", err)
	}

	shared.RespondEphemeral(s, i, fmt.Sprintf("반복 모드를 %s으로 설정했습니다.", label))
}

func getSubcommandOption(data discordgo.ApplicationCommandInteractionData) *discordgo.ApplicationCommandInteractionDataOption {
	for _, opt := range data.Options {
		if opt.Type == discordgo.ApplicationCommandOptionSubCommand {
			return opt
		}
	}
	return nil
}

func GetInteractionAwaiter() *modals.Awaiter {
	return modals.DefaultAwaiter
}

func RegisterCommands(s *discordgo.Session, appID string, guildID string) ([]*discordgo.ApplicationCommand, error) {
	scope := "global"
	if guildID != "" {
		scope = fmt.Sprintf("guild:%s", guildID)
	}

	log.Printf("Registering %d commands (%s)", len(CommandList), scope)

	cmds, err := s.ApplicationCommandBulkOverwrite(appID, guildID, CommandList)
	if err != nil {
		return nil, fmt.Errorf("cannot bulk overwrite commands: %w", err)
	}
	return cmds, nil
}

func AddHandlers(s *discordgo.Session) {
	s.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		musiclisteners.HandleMusicMessage(s, m)
	})

	s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if modals.DefaultAwaiter.HandleInteraction(i) {
			return
		}

		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			data := i.ApplicationCommandData()
			if handler, ok := commandHandlers[data.Name]; ok {
				handler(s, i)
			}
		case discordgo.InteractionModalSubmit:
			if dashboardlisteners.RouteDashboardComponent(s, i) {
				return
			}
		case discordgo.InteractionMessageComponent:
			if pinglisteners.RoutePingComponent(s, i) {
				return
			}
			if musiclisteners.RouteMusicComponent(s, i) {
				return
			}
			if dashboardlisteners.RouteDashboardComponent(s, i) {
				return
			}
		default:
			return
		}
	})
}

package commands

import (
	import (
	"fmt"
	"strings"
	"os"      // ထည့်ရန်
	"strconv" // ထည့်ရန်
	"math/rand" // ထည့်ရန်
	"EverythingSuckz/fsb/config"
	"EverythingSuckz/fsb/internal/utils"

	"github.com/celestix/gotgproto/dispatcher"
	"github.com/celestix/gotgproto/dispatcher/handlers"
	"github.com/celestix/gotgproto/ext"
	"github.com/celestix/gotgproto/storage"
	"github.com/celestix/gotgproto/types"
	"github.com/gotd/td/telegram/message/styling"
	"github.com/gotd/td/tg"
)

func (m *command) LoadStream(dispatcher dispatcher.Dispatcher) {
	log := m.log.Named("start")
	defer log.Sugar().Info("Loaded")
	dispatcher.AddHandler(
		handlers.NewMessage(nil, sendLink),
	)
}

func supportedMediaFilter(m *types.Message) (bool, error) {
	if not := m.Media == nil; not {
		return false, dispatcher.EndGroups
	}
	switch m.Media.(type) {
	case *tg.MessageMediaDocument:
		return true, nil
	case *tg.MessageMediaPhoto:
		return true, nil
	case tg.MessageMediaClass:
		return false, dispatcher.EndGroups
	default:
		return false, nil
	}
}

func sendLink(ctx *ext.Context, u *ext.Update) error {
	chatId := u.EffectiveChat().GetID()
	peerChatId := ctx.PeerStorage.GetPeerById(chatId)
	if peerChatId.Type != int(storage.TypeUser) {
		return dispatcher.EndGroups
	}
	if len(config.ValueOf.AllowedUsers) != 0 && !utils.Contains(config.ValueOf.AllowedUsers, chatId) {
		ctx.Reply(u, ext.ReplyTextString("You are not allowed to use this bot."), nil)
		return dispatcher.EndGroups
	}
	supported, err := supportedMediaFilter(u.EffectiveMessage)
	if err != nil {
		return err
	}
	if !supported {
		ctx.Reply(u, ext.ReplyTextString("Sorry, this message type is unsupported."), nil)
		return dispatcher.EndGroups
	}
	// (၁) Log Channel ဆီ ပို့ခြင်း (Link ထုတ်ရန်အတွက်)
	update, err := utils.ForwardMessages(ctx, chatId, config.ValueOf.LogChannelID, u.EffectiveMessage.ID)
	if err != nil {
		utils.Logger.Sugar().Error(err)
		ctx.Reply(u, ext.ReplyTextString(fmt.Sprintf("Error - %s", err.Error())), nil)
		return dispatcher.EndGroups
	}

	// (၂) Backup Channel ဆီ ပို့ခြင်း
	backupEnv := os.Getenv("BACKUP_CHANNEL")
	if backupEnv != "" {
		cleanBID, pErr := strconv.ParseInt(strings.TrimPrefix(backupEnv, "-100"), 10, 64)
		if pErr == nil {
			go func(bID int64) {
				// အရင်ဆုံး Backup Channel ရဲ့ Access Hash ကို သိအောင် API နဲ့ အရင်ရှာမယ်
				inputChannel := &tg.InputChannel{ChannelID: bID}
				res, bErr := ctx.Raw.ChannelsGetChannels(ctx, []tg.InputChannelClass{inputChannel})
				
				if bErr == nil && len(res.GetChats()) > 0 {
					if channel, ok := res.GetChats()[0].(*tg.Channel); ok {
						// Access Hash ရပြီဆိုတော့မှ Forward လုပ်မယ်
						ctx.Raw.MessagesForwardMessages(ctx, &tg.MessagesForwardMessagesRequest{
							DropAuthor: true,
							RandomID:   []int64{rand.Int63()},
							FromPeer:   u.EffectiveMessage.GetInputPeer(),
							ID:         []int{u.EffectiveMessage.ID},
							ToPeer:     channel.AsInput(),
						})
					}
				} else if bErr != nil {
					utils.Logger.Sugar().Errorf("Backup Search Error: %v", bErr)
				}
			}(cleanBID)
		}
	}
	// ------------------------------------------
	if strings.Contains(file.MimeType, "video") || strings.Contains(file.MimeType, "audio") || strings.Contains(file.MimeType, "pdf") {
		row.Buttons = append(row.Buttons, &tg.KeyboardButtonURL{
			Text: "Stream",
			URL:  link,
		})
	}
	markup := &tg.ReplyInlineMarkup{
		Rows: []tg.KeyboardButtonRow{row},
	}
	if strings.Contains(link, "http://localhost") {
		_, err = ctx.Reply(u, ext.ReplyTextStyledText(text), &ext.ReplyOpts{
			NoWebpage:        false,
			ReplyToMessageId: u.EffectiveMessage.ID,
		})
	} else {
		_, err = ctx.Reply(u, ext.ReplyTextStyledText(text), &ext.ReplyOpts{
			Markup:           markup,
			NoWebpage:        false,
			ReplyToMessageId: u.EffectiveMessage.ID,
		})
	}
	if err != nil {
		utils.Logger.Sugar().Error(err)
		ctx.Reply(u, ext.ReplyTextString(fmt.Sprintf("Error - %s", err.Error())), nil)
	}
	return dispatcher.EndGroups
}

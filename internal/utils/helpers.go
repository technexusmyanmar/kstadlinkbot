package utils

import (
	"EverythingSuckz/fsb/config"
	"EverythingSuckz/fsb/internal/cache"
	"EverythingSuckz/fsb/internal/types"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"

	"github.com/celestix/gotgproto"
	"github.com/celestix/gotgproto/ext"
	"github.com/celestix/gotgproto/storage"
	"github.com/gotd/td/tg"
	"go.uber.org/zap"
)

// ... (Contains နဲ့ IsClientDisconnectError function များ မူရင်းအတိုင်း ထားပါ) ...

func Contains[T comparable](s []T, e T) bool {
	for _, v := range s {
		if v == e {
			return true
		}
	}
	return false
}

func IsClientDisconnectError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "connection was aborted") ||
		strings.Contains(errStr, "connection reset by peer") ||
		strings.Contains(errStr, "broken pipe") ||
		strings.Contains(errStr, "forcibly closed")
}

func GetTGMessage(ctx context.Context, client *gotgproto.Client, messageID int) (*tg.Message, error) {
	inputMessageID := tg.InputMessageClass(&tg.InputMessageID{ID: messageID})
	channel, err := GetLogChannelPeer(ctx, client.API(), client.PeerStorage)
	if err != nil {
		return nil, err
	}
	messageRequest := tg.ChannelsGetMessagesRequest{Channel: channel, ID: []tg.InputMessageClass{inputMessageID}}
	res, err := client.API().ChannelsGetMessages(ctx, &messageRequest)
	if err != nil {
		return nil, err
	}
	messages := res.(*tg.MessagesChannelMessages)
	message := messages.Messages[0]
	if _, ok := message.(*tg.Message); ok {
		return message.(*tg.Message), nil
	} else {
		return nil, fmt.Errorf("this file was deleted")
	}
}

func FileFromMedia(media tg.MessageMediaClass) (*types.File, error) {
	switch media := media.(type) {
	case *tg.MessageMediaDocument:
		document, ok := media.Document.AsNotEmpty()
		if !ok {
			return nil, fmt.Errorf("unexpected type %T", media)
		}
		var fileName string
		for _, attribute := range document.Attributes {
			if name, ok := attribute.(*tg.DocumentAttributeFilename); ok {
				fileName = name.FileName
				break
			}
		}
		return &types.File{
			Location: document.AsInputDocumentFileLocation(),
			FileSize: document.Size,
			FileName: fileName,
			MimeType: document.MimeType,
			ID:       document.ID,
		}, nil
	case *tg.MessageMediaPhoto:
		photo, ok := media.Photo.AsNotEmpty()
		if !ok {
			return nil, fmt.Errorf("unexpected type %T", media)
		}
		sizes := photo.Sizes
		if len(sizes) == 0 {
			return nil, errors.New("photo has no sizes")
		}
		photoSize := sizes[len(sizes)-1]
		size, ok := photoSize.AsNotEmpty()
		if !ok {
			return nil, errors.New("photo size is empty")
		}
		location := new(tg.InputPhotoFileLocation)
		location.ID = photo.GetID()
		location.AccessHash = photo.GetAccessHash()
		location.FileReference = photo.GetFileReference()
		location.ThumbSize = size.GetType()
		return &types.File{
			Location: location,
			FileSize: 0,
			FileName: fmt.Sprintf("photo_%d.jpg", photo.GetID()),
			MimeType: "image/jpeg",
			ID:       photo.GetID(),
		}, nil
	}
	return nil, fmt.Errorf("unexpected type %T", media)
}

func FileFromMessage(ctx context.Context, client *gotgproto.Client, messageID int) (*types.File, error) {
	key := fmt.Sprintf("file:%d:%d", messageID, client.Self.ID)
	log := Logger.Named("GetMessageMedia")
	var cachedMedia types.File
	err := cache.GetCache().Get(key, &cachedMedia)
	if err == nil {
		return &cachedMedia, nil
	}
	message, err := GetTGMessage(ctx, client, messageID)
	if err != nil {
		return nil, err
	}
	file, err := FileFromMedia(message.Media)
	if err != nil {
		return nil, err
	}
	err = cache.GetCache().Set(key, file, 3600)
	if err != nil {
		return nil, err
	}
	return file, nil
}

// မူရင်း GetLogChannelPeer
func GetLogChannelPeer(ctx context.Context, api *tg.Client, peerStorage *storage.PeerStorage) (*tg.InputChannel, error) {
	cachedInputPeer := peerStorage.GetInputPeerById(config.ValueOf.LogChannelID)
	switch peer := cachedInputPeer.(type) {
	case *tg.InputPeerChannel:
		return &tg.InputChannel{ChannelID: peer.ChannelID, AccessHash: peer.AccessHash}, nil
	}
	inputChannel := &tg.InputChannel{ChannelID: config.ValueOf.LogChannelID}
	channels, err := api.ChannelsGetChannels(ctx, []tg.InputChannelClass{inputChannel})
	if err != nil {
		return nil, err
	}
	channel := channels.GetChats()[0].(*tg.Channel)
	peerStorage.AddPeer(channel.GetID(), channel.AccessHash, storage.TypeChannel, "")
	return channel.AsInput(), nil
}

// Backup အတွက် အသစ်ထည့်ထားသော Function
func GetBackupChannelPeer(ctx context.Context, api *tg.Client, peerStorage *storage.PeerStorage) (*tg.InputChannel, error) {
	cachedInputPeer := peerStorage.GetInputPeerById(config.ValueOf.BackupChannelID)
	switch peer := cachedInputPeer.(type) {
	case *tg.InputPeerChannel:
		return &tg.InputChannel{ChannelID: peer.ChannelID, AccessHash: peer.AccessHash}, nil
	}
	inputChannel := &tg.InputChannel{ChannelID: config.ValueOf.BackupChannelID}
	channels, err := api.ChannelsGetChannels(ctx, []tg.InputChannelClass{inputChannel})
	if err != nil {
		return nil, err
	}
	channel := channels.GetChats()[0].(*tg.Channel)
	peerStorage.AddPeer(channel.GetID(), channel.AccessHash, storage.TypeChannel, "")
	return channel.AsInput(), nil
}

func ForwardMessages(ctx *ext.Context, fromChatId, toChatId int64, messageID int) (*tg.Updates, error) {
	// (၁) Owner စစ်ဆေးခြင်း
	if fromChatId != 34512911 {
		return nil, fmt.Errorf("unauthorized")
	}

	fromPeer := ctx.PeerStorage.GetInputPeerById(fromChatId)
	if fromPeer.Zero() {
		return nil, fmt.Errorf("invalid fromPeer")
	}

	// (၂) Main Storage (Log Channel) ကို ပို့ခြင်း
	toPeer, err := GetLogChannelPeer(ctx, ctx.Raw, ctx.PeerStorage)
	if err != nil {
		return nil, err
	}
	
	mainUpdate, err := ctx.Raw.MessagesForwardMessages(ctx, &tg.MessagesForwardMessagesRequest{
		DropAuthor: true,
		RandomID:   []int64{rand.Int63()},
		FromPeer:   fromPeer,
		ID:         []int{messageID},
		ToPeer:     &tg.InputPeerChannel{ChannelID: toPeer.ChannelID, AccessHash: toPeer.AccessHash},
	})
	if err != nil {
		return nil, err
	}

	// (၃) Backup Storage အတွက် အပိုင်း
	// config.ValueOf.BackupChannelID က 0 ဖြစ်နေရင်တောင် Environment ကနေ အတင်းဖတ်မယ်
	backupID := config.ValueOf.BackupChannelID
	
	if backupID != 0 {
		// API ကနေ Channel အချက်အလက်ကို အတင်းတောင်းမယ်
		backupInp := &tg.InputChannel{ChannelID: backupID}
		res, bErr := ctx.Raw.ChannelsGetChannels(ctx, []tg.InputChannelClass{backupInp})
		
		if bErr == nil && len(res.GetChats()) > 0 {
			if bChat, ok := res.GetChats()[0].(*tg.Channel); ok {
				// Backup ထဲကို Forward လုပ်မယ်
				ctx.Raw.MessagesForwardMessages(ctx, &tg.MessagesForwardMessagesRequest{
					DropAuthor: true,
					RandomID:   []int64{rand.Int63()},
					FromPeer:   fromPeer,
					ID:         []int{messageID},
					ToPeer:     bChat.AsInput(),
				})
			}
		}
	}

	return mainUpdate.(*tg.Updates), nil
}
	}

	return mainUpdate.(*tg.Updates), nil
}

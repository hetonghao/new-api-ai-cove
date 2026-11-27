package service

import (
	"context"
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// ErrDeviMonChatGPTAccountMissing reports that an account has no stored
// sign-in yet, so it cannot be read through the ChatGPT channel.
var ErrDeviMonChatGPTAccountMissing = model.ErrDeviMonChatGPTAccountMissing

// DeviMonChatGPTAccountView is what the account dialog shows. Tokens stay out
// of the response; only the channel switch and sign-in state are exposed.
type DeviMonChatGPTAccountView struct {
	Email     string `json:"email"`
	AccountID string `json:"account_id"`
	Channel   string `json:"channel"`
	SignedIn  bool   `json:"signed_in"`
	ProxyURL  string `json:"proxy_url"`
	ExpiresAt int64  `json:"expires_at"`
}

func GetDeviMonChatGPTAccounts(ctx context.Context) ([]DeviMonChatGPTAccountView, error) {
	rows, err := model.GetDeviMonChatGPTAccounts(ctx)
	if err != nil {
		return nil, err
	}
	views := make([]DeviMonChatGPTAccountView, 0, len(rows))
	for _, row := range rows {
		views = append(views, DeviMonChatGPTAccountView{
			Email:     row.Email,
			AccountID: row.AccountID,
			Channel:   row.Channel,
			SignedIn:  row.AccessToken != "",
			ProxyURL:  row.ProxyURL,
			ExpiresAt: row.ExpiresAt,
		})
	}
	return views, nil
}

// SetDeviMonAccountChannel moves one account between the CPA inventory and the
// gateway-stored ChatGPT sign-in. Moving to ChatGPT requires a stored sign-in.
func SetDeviMonAccountChannel(ctx context.Context, email, channel string) error {
	if channel != model.DeviMonChannelCPA && channel != model.DeviMonChannelChatGPT {
		return errors.New("unknown subscription channel")
	}
	if channel == model.DeviMonChannelChatGPT {
		if _, err := model.GetDeviMonChatGPTAccount(ctx, email); err != nil {
			return ErrDeviMonChatGPTAccountMissing
		}
	}
	if _, err := model.SetDeviMonChatGPTChannel(ctx, email, channel); err != nil {
		return err
	}
	return nil
}

// SetDeviMonAccountProxy stores this account's egress proxy. The URL is
// validated before it is written so a bad value cannot break the next read.
func SetDeviMonAccountProxy(ctx context.Context, email, proxyURL string) error {
	proxyURL = strings.TrimSpace(proxyURL)
	if _, err := common.ParseProxyURLStrict(proxyURL); err != nil {
		return ErrDeviMonInvalidProxy
	}
	if _, err := model.GetDeviMonChatGPTAccount(ctx, email); err != nil {
		return ErrDeviMonChatGPTAccountMissing
	}
	if _, err := model.SetDeviMonChatGPTProxy(ctx, email, proxyURL); err != nil {
		return err
	}
	return nil
}

// ErrDeviMonInvalidProxy marks a proxy URL the operator can fix.
var ErrDeviMonInvalidProxy = errors.New("proxy URL must use http, https, socks5, or socks5h")

// ForgetDeviMonChatGPTAccount drops the stored sign-in, returning the account
// to the CPA channel.
func ForgetDeviMonChatGPTAccount(ctx context.Context, email string) error {
	return model.DeleteDeviMonChatGPTAccount(ctx, email)
}

package corplink

import (
	"context"
	"fmt"
	"time"
)

// SendCode sends a verification code to email or phone.
// codeType: "email" or "phone"
func (c *Client) SendCode(ctx context.Context, codeType, account string) error {
	type req struct {
		ForgetPassword bool   `json:"forget_password"`
		CodeType       string `json:"code_type"`
		UserName       string `json:"user_name"`
	}
	var resp apiResp[any]
	if err := c.post(ctx, c.apiURL("/api/login/code/send"), req{
		ForgetPassword: false,
		CodeType:       codeType,
		UserName:       account,
	}, &resp); err != nil {
		return err
	}
	if resp.Code != 0 {
		return fmt.Errorf("send code: %s", resp.Message)
	}
	return nil
}

// VerifyCode submits a verification code and completes login.
// On success the TOTP secret is extracted from the response and saved to the session.
func (c *Client) VerifyCode(ctx context.Context, codeType, account, code string) error {
	type req struct {
		ForgetPassword bool   `json:"forget_password"`
		CodeType       string `json:"code_type"`
		UserName       string `json:"user_name"`
		Code           string `json:"code"`
	}
	type verifyRespData struct {
		URL string `json:"url"`
	}
	var resp apiResp[verifyRespData]
	if err := c.post(ctx, c.apiURL("/api/login/code/verify"), req{
		ForgetPassword: false,
		CodeType:       codeType,
		UserName:       account,
		Code:           code,
	}, &resp); err != nil {
		return err
	}
	if resp.Code != 0 {
		return fmt.Errorf("verify code: %s", resp.Message)
	}
	c.extractAndSaveTOTPSecret(ctx, resp.Data.URL)
	return nil
}

// QRCodeResult holds data for QR code login flow.
type QRCodeResult struct {
	LoginURL string
	Token    string
}

// GetQRCode fetches a Lark/OIDC OAuth login URL and polling token.
func (c *Client) GetQRCode(ctx context.Context) (*QRCodeResult, error) {
	// data is an array of TPS items; find the first (lark) entry.
	type tpsItem struct {
		AliasKey string `json:"alias_key"`
		LoginURL string `json:"login_url"`
		Token    string `json:"token"`
	}
	var result apiResp[[]tpsItem]
	if err := c.get(ctx, c.apiURL("/api/tpslogin/link"), &result); err != nil {
		return nil, err
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("get qr code: %s", result.Message)
	}
	if len(result.Data) == 0 {
		return nil, fmt.Errorf("get qr code: no TPS items returned")
	}
	item := result.Data[0]
	return &QRCodeResult{LoginURL: item.LoginURL, Token: item.Token}, nil
}

// PollQRLogin polls until QR code is scanned and login is complete.
// On success the TOTP secret is extracted from the response and saved to the session.
func (c *Client) PollQRLogin(ctx context.Context, token string) error {
	type req struct {
		Token string `json:"token"`
	}
	type qrRespData struct {
		URL string `json:"url"`
	}
	deadline := time.Now().Add(3 * time.Minute)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("QR login timed out after 3 minutes")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
		var resp apiResp[qrRespData]
		if err := c.post(ctx, c.apiURL("/api/tpslogin/token/check"), req{Token: token}, &resp); err != nil {
			continue
		}
		if resp.Code == 0 {
			c.extractAndSaveTOTPSecret(ctx, resp.Data.URL)
			return nil
		}
		if resp.Code == 101 {
			continue // not yet scanned
		}
		return fmt.Errorf("qr login error: %s", resp.Message)
	}
}

// Logout logs out the current session.
func (c *Client) Logout(ctx context.Context) {
	var resp apiResp[any]
	c.get(ctx, c.apiURL("/api/logout")+"&logout_all=false", &resp) //nolint:errcheck
	c.session.mu.Lock()
	c.session.Cookies = nil
	c.session.TOTPSecret = ""
	c.session.rebuildJar()
	c.session.mu.Unlock()
}

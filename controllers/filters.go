package controllers

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/astaxie/beego/context"
	"github.com/laincloud/mysql-service/monitor"
)

type ConsoleRole struct {
	Role string `json:"role"`
}

type ConsoleAuthResponse struct {
	Message string      `json:"msg"`
	URL     string      `json:"url"`
	Role    ConsoleRole `json:"role"`
}

// FilterConsoleLogin prohabits those unauthorized requests
func FilterConsoleLogin(ctx *context.Context) {
	//检查是否开启了SSO验证
	var authConf monitor.AuthConfInfo
	if data, err := monitor.GetLainConf("auth/console"); err == nil {
		tmpMap := make(map[string]string)
		json.Unmarshal(data, &tmpMap)
		if str, exist := tmpMap["auth/console"]; exist {
			json.Unmarshal([]byte(str), &authConf)
		}
	}

	//如果没有开启SSO验证，则跳过后面的验证逻辑
	if authConf.Type != "lain-sso" {
		return
	}

	//如果Session中没有access_token或者有但是验证不通过，则跳转到sso的登录页面
	if token, exist := ctx.Input.Session("access_token").(string); exist {
		if !validateConsoleRole(monitor.ConsoleAuthURL, token) {
			redirectToSSO(ctx, &authConf)
		}
	} else if code := ctx.Input.Query("code"); code == "" {
		redirectToSSO(ctx, &authConf)
	} else {
		//如果没有access_token但是有code，则用该code去sso获取access_token
		v := url.Values{}
		v.Set("code", code)
		v.Set("client_id", monitor.SecretConf["client_id"])
		v.Set("client_secret", monitor.SecretConf["secret"])
		v.Set("redirect_uri", monitor.SSORedirectURI)
		v.Set("grant_type", "authorization_code")
		client := http.DefaultClient
		var (
			err       error
			resp      *http.Response
			respBytes []byte
		)
		resp, err = client.Get(fmt.Sprintf("%s/oauth2/token?%s", authConf.URL, v.Encode()))
		if err == nil {
			defer resp.Body.Close()
			if respBytes, err = ioutil.ReadAll(resp.Body); err == nil {
				caResp := make(map[string]interface{})
				if err = json.Unmarshal(respBytes, &caResp); err == nil {
					if token, exists := caResp["access_token"]; exists {
						ctx.Output.Session("access_token", token.(string))
						ctx.Redirect(http.StatusFound, "/")
					} else {
						err = fmt.Errorf("No token find in the response")
					}
				}
			}
		}
		if err != nil {
			v := url.Values{}
			v.Set("errNo", strconv.Itoa(http.StatusUnauthorized))
			v.Set("errTitle", "Authorize failed")
			v.Set("errMsg", err.Error())
			ctx.Redirect(http.StatusUnauthorized, "/error?%s"+v.Encode())
		}
	}
}

func validateConsoleRole(authURL, token string) bool {
	client := http.DefaultClient
	if req, err := http.NewRequest("GET", authURL, nil); err == nil {
		req.Header.Set("access-token", token)
		if resp, err := client.Do(req); err == nil {
			defer resp.Body.Close()
			if respBytes, err := ioutil.ReadAll(resp.Body); err == nil {
				caResp := ConsoleAuthResponse{}
				return json.Unmarshal(respBytes, &caResp) == nil && caResp.Role.Role != ""
			}
		}
	}
	return false
}

func redirectToSSO(ctx *context.Context, authConf *monitor.AuthConfInfo) {
	v := url.Values{}
	v.Set("response_type", "code")
	v.Set("redirect_uri", monitor.SSORedirectURI)
	v.Set("realm", "mysql")
	v.Set("client_id", monitor.SecretConf["client_id"])
	v.Set("scope", "write:group")
	v.Set("state", fmt.Sprintf("%d", time.Now().Unix()))
	ctx.Redirect(302, fmt.Sprintf("%s/oauth2/auth?%s", authConf.URL, v.Encode()))
}

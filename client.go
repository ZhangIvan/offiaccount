// Copyright 2020 FastWeGo
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package offiaccount

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

/*
微信 api 服务器地址
*/
var (
	WXServerUrl                 = "https://api.weixin.qq.com"
	NeedRefreshAccessTokenError = errors.New("access token invalid")
	WXInvalidErrorCode          = map[int64]string{40001: "access_token 无效", 40014: "不合法的access_token"}
)

/*
HttpClient 用于向公众号接口发送请求
*/
type Client struct {
	Ctx *OffiAccount
}

// HTTPGet GET 请求
func (client *Client) HTTPGet(uri string) (resp []byte, err error) {
	uri, err = client.applyAccessToken(uri)
	if err != nil {
		return
	}
	if client.Ctx.Logger != nil {
		client.Ctx.Logger.Printf("GET %s", uri)
	}
	response, err := http.Get(WXServerUrl + uri)
	if err != nil {
		return
	}
	defer response.Body.Close()
	resp, err = responseFilter(response)

	if err == NeedRefreshAccessTokenError {
		_, err := client.Ctx.AccessToken.GetRefreshAccessTokenHandler(client.Ctx)
		if err != nil {
			return
		}

		uri, err = client.applyAccessToken(uri)
		if err != nil {
			return
		}
		if client.Ctx.Logger != nil {
			client.Ctx.Logger.Printf("Refresh Access Token Second GET %s", uri)
		}
		response, err := http.Get(WXServerUrl + uri)
		if err != nil {
			return
		}
		defer response.Body.Close()
		return responseFilter(response)
	}
	return
}

//HTTPPost POST 请求
func (client *Client) HTTPPost(uri string, payload io.Reader, contentType string) (resp []byte, err error) {
	uri, err = client.applyAccessToken(uri)
	if err != nil {
		return
	}
	if client.Ctx.Logger != nil {
		client.Ctx.Logger.Printf("POST %s", uri)
	}
	response, err := http.Post(WXServerUrl+uri, contentType, payload)
	if err != nil {
		return
	}
	defer response.Body.Close()
	resp, err = responseFilter(response)

	if err == NeedRefreshAccessTokenError {
		_, err := client.Ctx.AccessToken.GetRefreshAccessTokenHandler(client.Ctx)
		if err != nil {
			return
		}

		uri, err = client.applyAccessToken(uri)
		if err != nil {
			return
		}
		if client.Ctx.Logger != nil {
			client.Ctx.Logger.Printf("Refresh Access Token Second POST %s", uri)
		}
		response, err := http.Post(WXServerUrl+uri, contentType, payload)
		if err != nil {
			return
		}
		defer response.Body.Close()
		return responseFilter(response)
	}
	return
}

/*
在请求地址上附加上 access_token
*/
func (client *Client) applyAccessToken(oldUrl string) (newUrl string, err error) {
	accessToken, err := client.Ctx.AccessToken.GetAccessTokenHandler(client.Ctx)
	if err != nil {
		return
	}
	if strings.Contains(oldUrl, "?") {
		newUrl = oldUrl + "&access_token=" + accessToken
	} else {
		newUrl = oldUrl + "?access_token=" + accessToken
	}
	return
}

/*
筛查微信 api 服务器响应，判断以下错误：

- http 状态码 不为 200

- 接口响应错误码 errcode 不为 0
*/
func responseFilter(response *http.Response) (resp []byte, err error) {
	if response.StatusCode != http.StatusOK {
		err = fmt.Errorf("Status %s", response.Status)
		return
	}

	resp, err = ioutil.ReadAll(response.Body)
	if err != nil {
		return
	}

	errorResponse := struct {
		Errcode int64  `json:"errcode"`
		Errmsg  string `json:"errmsg"`
	}{}
	err = json.Unmarshal(resp, &errorResponse)
	if err != nil {
		return
	}

	if _, ok := WXInvalidErrorCode[errorResponse.Errcode]; ok {
		err = NeedRefreshAccessTokenError
		return
	}

	if errorResponse.Errcode != 0 {
		err = errors.New(string(resp))
		return
	}

	return
}

// 防止多个 goroutine 并发刷新冲突
var refreshAccessTokenLock sync.Mutex

/*
从 公众号实例 的 AccessToken 管理器 获取 access_token

如果没有 access_token 或者 已过期，那么刷新

获得新的 access_token 后 过期时间设置为 0.9 * expiresIn 提供一定冗余
*/
func GetAccessToken(ctx *OffiAccount) (accessToken string, err error) {
	accessToken, err = ctx.AccessToken.Cache.Fetch(ctx.Config.Appid)
	if accessToken != "" {
		return
	}

	refreshAccessTokenLock.Lock()
	defer refreshAccessTokenLock.Unlock()

	accessToken, err = ctx.AccessToken.Cache.Fetch(ctx.Config.Appid)
	if accessToken != "" {
		return
	}

	accessToken, expiresIn, err := refreshAccessTokenFromWXServer(ctx.Config.Appid, ctx.Config.Secret)
	if err != nil {
		return
	}

	// 提前过期 提供冗余时间
	expiresIn = int(0.9 * float64(expiresIn))
	d := time.Duration(expiresIn) * time.Second
	_ = ctx.AccessToken.Cache.Save(ctx.Config.Appid, accessToken, d)

	if ctx.Logger != nil {
		ctx.Logger.Printf("%s %s %d\n", "refreshAccessTokenFromWXServer", accessToken, expiresIn)
	}

	return
}

/*
从 公众号实例 的 AccessToken 管理器 更新 access_token

获得新的 access_token 后 过期时间设置为 0.9 * expiresIn 提供一定冗余
*/
func NoticeRefreshAccessToken(ctx *OffiAccount) (accessToken string, err error) {
	refreshAccessTokenLock.Lock()
	defer refreshAccessTokenLock.Unlock()

	accessToken, expiresIn, err := refreshAccessTokenFromWXServer(ctx.Config.Appid, ctx.Config.Secret)
	if err != nil {
		return
	}

	// 提前过期 提供冗余时间
	expiresIn = int(0.9 * float64(expiresIn))
	d := time.Duration(expiresIn) * time.Second
	_ = ctx.AccessToken.Cache.Save(ctx.Config.Appid, accessToken, d)

	if ctx.Logger != nil {
		ctx.Logger.Printf("%s %s %d\n", "refreshAccessTokenFromWXServer", accessToken, expiresIn)
	}

	// 可以做一些别的事情
	// 如：通知 中控服务，让中控处理更新自己的 accessToken， 中控接收到请求后，对比中控缓存的accessToken时候更新accessToken，注意并发读写问题
	// do something ...

	return

}

/*
从微信服务器获取新的 AccessToken

See: https://developers.weixin.qq.com/doc/offiaccount/Basic_Information/Get_access_token.html
*/
func refreshAccessTokenFromWXServer(appid string, secret string) (accessToken string, expiresIn int, err error) {
	params := url.Values{}
	params.Add("appid", appid)
	params.Add("secret", secret)
	params.Add("grant_type", "client_credential")
	url := WXServerUrl + "/cgi-bin/token?" + params.Encode()

	response, err := http.Get(url)
	if err != nil {
		return
	}

	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		err = fmt.Errorf("GET %s RETURN %s", url, response.Status)
		return
	}

	resp, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return
	}

	var result = struct {
		AccessToken string  `json:"access_token"`
		ExpiresIn   int     `json:"expires_in"`
		Errcode     float64 `json:"errcode"`
		Errmsg      string  `json:"errmsg"`
	}{}

	err = json.Unmarshal(resp, &result)
	if err != nil {
		err = fmt.Errorf("Unmarshal error %s", string(resp))
		return
	}

	if result.AccessToken == "" {
		err = fmt.Errorf("%s", string(resp))
		return
	}

	return result.AccessToken, result.ExpiresIn, nil
}

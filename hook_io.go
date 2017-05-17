package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	hookStorageAPIURL = "https://hook.io/datastore"
)

type hookDBClient struct {
	key    string
	client *http.Client
}

type responseError struct {
	Error bool   `json:"error"`
	Msg   string `json:"message"`
}

func newStorage(key string) *hookDBClient {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	return &hookDBClient{key: key, client: client}
}

func (c *hookDBClient) Get(key string) (value string, err error) {
	params := url.Values{
		"key":              []string{key},
		"hook_private_key": []string{c.key},
	}
	r, _ := http.NewRequest("GET", hookStorageAPIURL+"/get?"+params.Encode(), nil)

	var res *http.Response
	if res, err = c.client.Do(r); err != nil {
		return
	}
	defer res.Body.Close()

	buf := &bytes.Buffer{}
	if _, err = io.Copy(buf, res.Body); err != nil {
		return
	}

	if err = c.checkErrorResponse(buf.Bytes()); err != nil {
		return
	}

	value, err = strconv.Unquote(buf.String())
	if err != nil {
		value = buf.String()
		err = nil
	}

	return
}

func (c *hookDBClient) Set(key, value string) (err error) {
	data := url.Values{
		"key":              []string{key},
		"value":            []string{value},
		"hook_private_key": []string{c.key},
	}
	r, _ := http.NewRequest("POST", hookStorageAPIURL+"/set", strings.NewReader(data.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var res *http.Response
	if res, err = c.client.Do(r); err != nil {
		return
	}
	defer res.Body.Close()

	buf := &bytes.Buffer{}
	if _, err = io.Copy(buf, res.Body); err != nil {
		return
	}

	if err = c.checkErrorResponse(buf.Bytes()); err != nil {
		return
	}

	var resMsg string
	if resMsg, err = strconv.Unquote(buf.String()); err != nil {
		resMsg = buf.String()
		err = nil
	}

	if resMsg != "OK" {
		err = errors.New(resMsg)
	}

	return
}

func (c *hookDBClient) checkErrorResponse(data []byte) (err error) {
	res := responseError{}
	json.Unmarshal(data, &res)

	if res.Error {
		err = errors.New(res.Msg)
	}

	return
}

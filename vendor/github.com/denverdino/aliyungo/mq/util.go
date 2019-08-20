package mq

import (
	"bytes"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"

	"time"
)

// 获取当前时间戳(毫秒)
func GetCurrentMillisecond() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

// 获取当前时间戳(秒)
func GetCurrentUnixMicro() int64 {
	return time.Now().Unix() * 1000
}

// 对字符串进行sha1 计算
func Sha1(data string) string {
	t := sha1.New()
	io.WriteString(t, data)
	return fmt.Sprintf("%x", t.Sum(nil))
}

// 对数据进行md5计算
func Md5(byteMessage []byte) string {
	h := md5.New()
	h.Write(byteMessage)
	return hex.EncodeToString(h.Sum(nil))
}

func HamSha1(data string, key []byte) string {
	hmac := hmac.New(sha1.New, key)
	hmac.Write([]byte(data))

	return base64.StdEncoding.EncodeToString(hmac.Sum(nil))
}

func dump(data interface{}) {
	fmt.Println(data)
}

// POST请求
func httpPost(urlStr string, header map[string]string, body []byte) ([]byte, int, error) {

	client := &http.Client{
		Transport: &http.Transport{
			Dial: func(netw, addr string) (net.Conn, error) {
				conn, err := net.DialTimeout(netw, addr, time.Second*2)
				if err != nil {
					return nil, err
				}
				conn.SetDeadline(time.Now().Add(time.Second * 2))
				return conn, nil
			},
			ResponseHeaderTimeout: time.Second * 2,
		},
	}

	req, err := http.NewRequest("POST", urlStr, bytes.NewBuffer([]byte(body)))
	if err != nil {
		return []byte(""), 0, err
	}

	req.Header.Set("Content-Type", "text/html;charset=utf-8")
	// req.Header.Set("Content-Type", "application/x-www-form-urlencoded; param=value")
	for k, v := range header {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 && resp.StatusCode != 201 && resp.StatusCode != 204 {
		return nil, resp.StatusCode, nil
	}

	data, err := ioutil.ReadAll(resp.Body)
	return data, resp.StatusCode, err
}

// GET请求
func HttpGet(urlStr string, header map[string]string) ([]byte, int, error) {

	client := &http.Client{
		Transport: &http.Transport{
			Dial: func(netw, addr string) (net.Conn, error) {
				conn, err := net.DialTimeout(netw, addr, time.Second*30)
				if err != nil {
					return nil, err
				}
				conn.SetDeadline(time.Now().Add(time.Second * 30))
				return conn, nil
			},
			ResponseHeaderTimeout: time.Second * 30,
		},
	}
	req, _ := http.NewRequest("GET", urlStr, nil)

	req.Header.Set("Content-Type", "text/html;charset=utf-8")
	// req.Header.Set("Content-Type", "application/x-www-form-urlencoded; param=value")
	for k, v := range header {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 201 && resp.StatusCode != 204 {
		return nil, resp.StatusCode, nil
	}

	data, err := ioutil.ReadAll(resp.Body)
	return data, resp.StatusCode, err
}

// GET请求
func HttpDelete(urlStr string, header map[string]string) ([]byte, int, error) {

	client := &http.Client{
		Transport: &http.Transport{
			Dial: func(netw, addr string) (net.Conn, error) {
				conn, err := net.DialTimeout(netw, addr, time.Second*2)
				if err != nil {
					return nil, err
				}
				conn.SetDeadline(time.Now().Add(time.Second * 2))
				return conn, nil
			},
			ResponseHeaderTimeout: time.Second * 2,
		},
	}
	req, _ := http.NewRequest("DELETE", urlStr, nil)

	req.Header.Set("Content-Type", "text/html;charset=utf-8")
	for k, v := range header {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 201 && resp.StatusCode != 204 {
		return nil, resp.StatusCode, nil
	}

	data, err := ioutil.ReadAll(resp.Body)
	return data, resp.StatusCode, err
}

package oss

import (
	"crypto"
	"crypto/md5"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"
	"sync"
)

type authenticationType struct {
	lock        *sync.RWMutex
	certificate map[string]*rsa.PublicKey
}

var (
	authentication = authenticationType{lock: &sync.RWMutex{}, certificate: map[string]*rsa.PublicKey{}}
	urlReg         = regexp.MustCompile(`^http(|s)://gosspublic.alicdn.com/[0-9a-zA-Z]`)
)

//验证OSS向业务服务器发来的回调函数。
//该方法是并发安全的
//pubKeyUrl 回调请求头中[x-oss-pub-key-url]一项，以Base64编码
//reqUrl oss所发来请求的url，由path+query组成
//reqBody oss所发来请求的body
//authorization authorization为回调头中的签名
func AuthenticateCallBack(pubKeyUrl, reqUrl, reqBody, authorization string) error {
	//获取证书url
	keyURL, err := base64.URLEncoding.DecodeString(pubKeyUrl)
	if err != nil {
		return err
	}
	url := string(keyURL)
	//判断证书是否来自于阿里云
	if !urlReg.Match(keyURL) {
		return errors.New("certificate address error")
	}
	//获取文件名
	rs := []rune(url)
	filename := string(rs[strings.LastIndex(url, "/") : len(rs)-1])
	authentication.lock.RLock()
	certificate := authentication.certificate[filename]
	authentication.lock.RUnlock()
	//内存中没有证书，下载
	if certificate == nil {
		authentication.lock.Lock()
		res, err := http.Get(url)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return err
		}
		block, _ := pem.Decode(body)
		if block == nil {
			return errors.New("certificate error")
		}
		pubKey, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return err
		}
		certificate = pubKey.(*rsa.PublicKey)
		authentication.certificate[filename] = certificate
		authentication.lock.Unlock()
	}
	//证书准备完毕，开始验证
	//解析签名
	signature, err := base64.StdEncoding.DecodeString(authorization)
	if err != nil {
		return err
	}
	hashed := md5.New()
	hashed.Write([]byte(reqUrl + "\n" + reqBody))
	if err := rsa.VerifyPKCS1v15(certificate, crypto.MD5, hashed.Sum(nil), signature); err != nil {
		return err
	}
	//验证通过
	return nil
}

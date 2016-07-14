package main

import (
	"crypto/hmac"
	"encoding/base64"
	"fmt"
	"time"

	"bytes"
	"crypto/sha1"

	"github.com/valyala/fasthttp"
)

type nullAuther struct {
}

func (a *nullAuther) sign(r *fasthttp.Request) {

}

type s3Auther struct {
	secretKey string
	apiKey    string
	bucket    string
	endpoint  string
}

func (a *s3Auther) sign(r *fasthttp.Request) {
	// TODO: This first part of func is just wrong,
	// need to refactor into some HTTPRequestFlavorer with s3 flavor
	// and this is must be done for implementing s3 V4 auth support
	date := time.Now().UTC().Format(time.RFC1123)
	bucketKey := fmt.Sprintf("%s/%s", a.bucket, r.RequestURI())
	r.SetRequestURI(fmt.Sprintf("http://%s/%s", a.endpoint, bucketKey))
	r.Header.Set("Date", date)
	r.Header.Set("Content-Type", "text/html")

	if config.S3ApiKey == NotSetString && config.S3SecretKey == NotSetString {
		return
	}
	mac := hmac.New(sha1.New, []byte(a.secretKey))
	stringToSign := bytes.Buffer{}
	stringToSign.Write([]byte(r.Header.Method()))
	stringToSign.Write([]byte("\n\n"))
	stringToSign.Write([]byte("text/html\n"))
	stringToSign.Write([]byte(date))
	stringToSign.Write([]byte("\n"))
	stringToSign.Write([]byte("/"))
	stringToSign.Write([]byte(bucketKey))
	mac.Write(stringToSign.Bytes())
	sha := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	r.Header.Set("Authorization", fmt.Sprintf("AWS %v:%v", a.apiKey, sha))
}

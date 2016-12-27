package main

import (
	"crypto/hmac"
	"encoding/base64"
	"fmt"
	"time"

	"bytes"
	"crypto/sha1"

	"github.com/valyala/fasthttp"
	"encoding/hex"
	"crypto/sha256"
)

type nullAuther struct {
}

func (a *nullAuther) sign(r *fasthttp.Request) {

}

type s3Params struct{
	secretKey string
	apiKey    string
	bucket    string
	endpoint  string
}

type s3AutherV2 struct {
	s3Params
}

type s3AutherV4 struct {
	s3Params
}

func sign(payload []byte, key []byte) []byte{
	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	return mac.Sum(nil)
}

func signSha1(payload []byte, key []byte) []byte{
	mac := hmac.New(sha1.New, key)
	mac.Write(payload)
	return mac.Sum(nil)
}

func getHash(payload []byte, key []byte) string{
	signed := sign(payload, key)
	return hex.EncodeToString(signed)
}

func getSha256(payload []byte) string{
	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:])
}

func (a *s3AutherV4) sign(r *fasthttp.Request) {
	// TODO: This first part of func is just wrong,
	// need to refactor into some HTTPRequestFlavorer with s3 flavor
	// and this is must be done for implementing s3 V4 auth support
	date := time.Now().UTC()
	dateFull := date.Format("20060102T150405Z")
	dateDate := date.Format("20060102")
	bucketKey := fmt.Sprintf("%s/%s", a.bucket, r.RequestURI())
	r.SetRequestURI(fmt.Sprintf("http://%s/%s", a.endpoint, bucketKey))
	r.Header.Set("x-amz-date", dateFull)
	r.Header.Set("Content-Type", "text/html")
	if config.S3ApiKey == NotSetString && config.S3SecretKey == NotSetString {
		return
	}
	var payload []byte
	if string(r.Header.Method()) == "GET" {
		payload = []byte("")
	}else{
		payload = r.Body()
	}
	payloadHash := getSha256(payload)
	r.Header.Set("X-Amz-Content-Sha256", payloadHash)

	canonicalUri := r.URI().Path()
	canonicalQueryString := ""
	canonicalHeaders := fmt.Sprintf("content-type:text/html\nhost:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n",
		r.URI().Host(), payloadHash, dateFull)
	signedHeaders := "content-type;host;x-amz-content-sha256;x-amz-date"
	canonicalRequest := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
		r.Header.Method(), canonicalUri, canonicalQueryString, canonicalHeaders, signedHeaders, payloadHash)
	algorithm := "AWS4-HMAC-SHA256"
	credentialScope := fmt.Sprintf("%s/%s/%s/aws4_request", dateDate, config.S3Region, "s3")
	stringToSign := fmt.Sprintf("%s\n%s\n%s\n%s",
		algorithm, dateFull, credentialScope, getSha256([]byte(canonicalRequest)))
	kDate := sign([]byte(dateDate), []byte(fmt.Sprintf("AWS4%s", config.S3SecretKey)))
	kRegion := sign([]byte(config.S3Region), kDate)
	kService := sign([]byte("s3"), kRegion)
	kSigning := sign([]byte("aws4_request"), kService)
	signature := getHash([]byte(stringToSign), kSigning)
	authHeader := fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		algorithm, config.S3ApiKey, credentialScope, signedHeaders, signature)
	r.Header.Set("Authorization", authHeader)
}

func (a *s3AutherV2) sign(r *fasthttp.Request) {
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
	stringToSign := bytes.Buffer{}
	stringToSign.Write([]byte(r.Header.Method()))
	stringToSign.Write([]byte("\n\n"))
	stringToSign.Write([]byte("text/html\n"))
	stringToSign.Write([]byte(date))
	stringToSign.Write([]byte("\n"))
	stringToSign.Write([]byte("/"))
	stringToSign.Write([]byte(bucketKey))
	b64Signed := base64.StdEncoding.EncodeToString(signSha1(stringToSign.Bytes(), []byte(a.secretKey)))
	r.Header.Set("Authorization", fmt.Sprintf("AWS %v:%v", a.apiKey, b64Signed))
}

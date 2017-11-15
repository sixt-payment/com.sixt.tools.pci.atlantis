package events

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"
)

type EcsCredentials struct {
	AccessKeyId     string
	Expiration      string
	RoleArn         string
	SecretAccessKey string
	Token           string
}

func handleEcsCredentials(relative_uri string) error {
	httpClient := &http.Client{Timeout: 5 * time.Second}
	url := fmt.Sprintf("http://169.254.170.2%s", relative_uri)
	r, err := httpClient.Get(url)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	credentials := &EcsCredentials{}
	err = json.NewDecoder(r.Body).Decode(credentials)
	if err != nil {
		return err
	}

	err = writeAwsCredentials(credentials)
	if err != nil {
		return err
	}

	return nil
}

func writeAwsCredentials(credentials *EcsCredentials) error {
	template := []string{
		"[default]",
		fmt.Sprintf("aws_access_key_id=%s", credentials.AccessKeyId),
		fmt.Sprintf("aws_secret_access_key=%s", credentials.SecretAccessKey),
		fmt.Sprintf("aws_session_token=%s", credentials.Token),
	}

	templateRendered := strings.Join(template, "\n")

	err := os.MkdirAll("/home/atlantis/.aws", os.FileMode(0700))
	if err != nil {
		return err
	}

	werr := ioutil.WriteFile("/home/atlantis/.aws/credentials", []byte(templateRendered), 0644)
	if werr != nil {
		return err
	}

	return nil
}

package yammer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strconv"
)

const (
	byEmailURL = "https://www.yammer.com/api/v1/users/by_email.json" // ?email=user@domain.com
	postURL    = "https://www.yammer.com/api/v1/messages.json"       // body replied_to_id
	//requestURL   = "https://www.yammer.com/api/v1/messages.json"
	inboxURL     = "https://www.yammer.com/api/v1/messages/inbox.json" // ?newer_than=[:message_id]
	followingURL = "https://www.yammer.com/api/v1/messages/following.json"
	receivedURL  = "https://www.yammer.com/api/v1/messages/received.json"
	privateURL   = "https://www.yammer.com/api/v1/messages/private.json"
)

// EmailToIDYammer email -> yammer id
func (y *Yammer) EmailToIDYammer(email string) (id int, err error) {
	r, inErr := y.transport.Client().Get(byEmailURL + "?email=" + email)
	if inErr != nil {
		log.Fatal("Get:", inErr)
		return 0, inErr
	}
	defer closePrint(r.Body)
	if r.StatusCode == 429 {
		err = fmt.Errorf("rate limit  %v", r.Status)
		log.Println(err)
		return
	}
	if r.StatusCode != 200 {
		err = fmt.Errorf("EmailToIDYammer err: %v", r.Status)
		log.Println(err)
		return
	}
	var data interface{}
	if err = json.NewDecoder(r.Body).Decode(&data); err != nil {
		return
	}
	id = int(data.([]interface{})[0].(map[string]interface{})["id"].(float64))
	fmt.Printf("--- id:\n%# v\n\n", id)
	return
}

// GetPrivate get private messages
func (y *Yammer) GetPrivate(lastID, limit int) ([]byte, error) {
	return y.getMessage(privateURL, lastID, limit)
}

// GetInbox get inbox messages
func (y *Yammer) GetInbox(lastID, limit int) ([]byte, error) {
	return y.getMessage(inboxURL, lastID, limit)
}

// GetFollowing get Following  messages
func (y *Yammer) GetFollowing(lastID, limit int) ([]byte, error) {
	return y.getMessage(followingURL, lastID, limit)
}

// GetReceived get received  messages
func (y *Yammer) GetReceived(lastID, limit int) ([]byte, error) {
	return y.getMessage(receivedURL, lastID, limit)
}

func (y *Yammer) getMessage(endpoint string, lastID, limit int) ([]byte, error) {
	urlValues := url.Values{}

	if lastID != 0 {
		urlValues.Add("newer_than", strconv.Itoa(lastID))
	}
	if limit != 0 {
		urlValues.Add("limit", strconv.Itoa(limit))
	}
	//pp.Print(endpoint + "?" + urlValues.Encode())
	r, inErr := y.transport.Client().Get(endpoint + "?" + urlValues.Encode())
	if inErr != nil {
		log.Fatalf("Get:%s", inErr)
		return nil, inErr
	}
	defer closePrint(r.Body)
	if r.StatusCode == 429 {
		err := fmt.Errorf("rate limit: %v", r.Status)
		log.Println(err)
		return nil, err
	}
	if r.StatusCode != 200 {
		err := fmt.Errorf("getMessage err: %v", r.Status)
		log.Println(err)
		return nil, err
	}
	return ioutil.ReadAll(r.Body)
}

func closePrint(c io.Closer) {
	if err := c.Close(); err != nil {
		log.Println(err)
	}
}

// Send message
func (y *Yammer) Send(method string, id int, message string) (string, error) {

	r, err := y.transport.Client().PostForm(postURL, url.Values{
		method: {strconv.Itoa(id)},
		"body": {message},
	})
	if err != nil {
		log.Fatalf("Get:%s", err)
		return "", err
	}
	defer closePrint(r.Body)
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(r.Body); err != nil {
		return buf.String(), fmt.Errorf("sendMessage err:%s", err)
	}
	if r.StatusCode != 200 {
		return buf.String(), fmt.Errorf("sendMessage Code:%d, Status:%v", r.StatusCode, r.Status)
	}
	return buf.String(), nil
}

// Unfollow unsubscribe thread
func (y *Yammer) Unfollow(id string) (string, error) {

	req, err := http.NewRequest("DELETE", "https://www.yammer.com/api/v1/threads/"+id+"/follow.json", nil)
	if err != nil {
		log.Fatalf("NewRequest:%s", err)
		return "", err
	}
	r, err := y.transport.Client().Do(req)
	if err != nil {
		log.Fatalf("Get:%s", err)
		return "", err
	}
	defer closePrint(r.Body)
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(r.Body); err != nil {
		return buf.String(), fmt.Errorf("Unsubscribe err:%s", err)
	}
	if r.StatusCode != 200 {
		return buf.String(), fmt.Errorf("Unsubscribe Code:%d, Status:%v", r.StatusCode, r.Status)
	}
	return buf.String(), nil
}

package main

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/masahide/go-yammer/cometd"
	"github.com/masahide/go-yammer/schema"
	"github.com/masahide/go-yammer/yammer"
	"github.com/nlopes/slack"
)

const (
	yammerFile    = "yammer.json"
	slackFile     = "slack.json"
	threadMapFile = "threadmap.json"
)

var (
	conf      Conf
	api       = slack.New(key)
	channels  = map[string]*slack.Channel{}
	threadMap = loadThreadMap(threadMapFile) // map[thread_id]slack_ts
	key       = loadSlackKey(slackFile)
	nameRep   = strings.NewReplacer(
		"(", "",
		")", "",
		".", "",
		"#", "",
		"$", "",
		"@", "",
		"%", "",
		"^", "",
		"&", "",
		"*", "",
		"+", "",
		"=", "",
		"[", "",
		"]", "",
		"{", "",
		"}", "",
		":", "",
		";", "",
		"'", "",
		"<", "",
		">", "",
		"/", "",
		"?", "",
		",", "",
		"|", "",
		"`", "",
		"\"", "",
	)
	debug   bool
	yClient *yammer.Client
	current *schema.User
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	flag.BoolVar(&debug, "debug", debug, "debug mode")
	flag.Parse()
	conf = loadConf(yammerFile)
	yClient = yammer.New(conf.AccessToken)
	yClient.DebugMode = debug
}
func getChannels() error {
	if len(channels) != 0 {
		return nil
	}
	chs, err := api.GetChannels(false)
	if err != nil {
		log.Println(err)
		return err
	}
	for i := range chs {
		channels[chs[i].Name] = &chs[i]
	}
	return nil
}

func main() {
	for {
		mainLoop()
	}
}
func mainLoop() {
	if err := getChannels(); err != nil {
		log.Println(err)
		return
	}
	realtime, err := yClient.Realtime()
	if err != nil {
		log.Println(err)
		return
	}
	current, err = yClient.Current()
	if err != nil {
		log.Println(err)
		return
	}
	inbox, err := yClient.InboxFeedV2()
	if err != nil {
		log.Println(err)
		return
	}

	rt := cometd.New(realtime.RealtimeURI, realtime.AuthenticationToken)
	err = rt.Handshake()
	if err != nil {
		log.Println(err)
		return
	}

	rt.SubscribeToFeed(inbox.ChannelID)
	messageChan := make(chan *cometd.ConnectionResponse, 10)
	stopChan := make(chan bool)

	log.Printf("Polling Realtime channelID: %v\n", inbox.ChannelID)
	go rt.Poll(messageChan, stopChan)
	for {
		select {
		case m, ok := <-messageChan:
			if !ok {
				return
			}
			if m.Channel == "/meta/connect" {
				continue
			}
			if m.Data.Type != "message" {
				log.Printf("Data.Type is not message. channel:%#v", m)
				continue
			}
			if m.Data.Feed == nil {
				log.Printf("Data.Feed is nil. channel:%#v", m)
				continue
			}
			receiveMessage(m.Data.Feed)
		}
		saveJSON(conf, yammerFile)
	}
}

func receiveMessage(feed *schema.MessageFeed) {
	for _, mes := range feed.Messages {
		//analysis(*mes, feed.References)
		if err := postMsg(*mes, feed.References); err != nil {
			log.Print(err)
			return
		}
		conf.InboxID = mes.Id
	}
}

func printClose(c io.Closer) {
	if err := c.Close(); err != nil {
		log.Println(err)
	}
}

func loadSlackKey(slackFile string) string {
	m := map[string]string{}
	f, err := os.Open(slackFile)
	if err != nil {
		log.Fatalf("Open %s err:%s", slackFile, err)
	}
	defer printClose(f)
	if err = json.NewDecoder(f).Decode(&m); err != nil {
		log.Fatalln(err)
	}
	k, ok := m["Key"]
	if !ok {
		log.Fatal("load slackFile err: not found 'Key'")
	}
	return k
}
func loadConf(file string) Conf {
	l := Conf{}
	f, err := os.Open(file)
	if err != nil {
		saveJSON(l, yammerFile)
		return l
	}
	defer printClose(f)
	if err = json.NewDecoder(f).Decode(&l); err != nil {
		log.Fatalln(err)
	}
	return l
}
func loadThreadMap(filename string) map[int]string {
	m := map[int]string{}
	f, err := os.Open(filename)
	if err != nil {
		saveJSON(m, threadMapFile)
		return m
	}
	defer printClose(f)
	if err = json.NewDecoder(f).Decode(&m); err != nil {
		log.Fatalln(err)
	}
	return m
}
func saveJSON(conf interface{}, file string) {
	f, err := os.Create(file)
	if err != nil {
		log.Fatal(err)
	}
	defer printClose(f)
	//b, err := json.Marshal(conf)
	b, err := json.MarshalIndent(conf, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if _, err = f.Write(b); err != nil {
		log.Fatal(err)
	}
}

// Conf save file
type Conf struct {
	// AccessToken  yammer access token
	AccessToken string
	// InboxID  inbox ID
	InboxID int
}

func nameHash(name string,size ,hsize int) string {
	if len(name) < size {
		return name
	}
	hasher := md5.New()
	hasher.Write([]byte(name))
	h := base64.StdEncoding.EncodeToString(hasher.Sum(nil))
	if len(h) > hsize {
		log.Faitalf("len(hash)>hsize,name:%s,hsize:%s,hash:%s",name,hsize ,h)
	}
	if hsize > size {
		return h[0:hsize]
	}
	return nameRep.Replace(name[0:size-hsize] + h[0:hsize])
}
/*
func nameHash(name string) string {
	if len(name) < 21 {
		return name
	}
	hasher := md5.New()
	hasher.Write([]byte(name))
	h := base64.StdEncoding.EncodeToString(hasher.Sum(nil))

	return nameRep.Replace(name[0:15] + h[0:6])
}
*/

func getRef(id int, refs []*schema.Reference) schema.Reference {
	for _, r := range refs {
		if r.ID == id {
			return *r
		}
	}
	return schema.Reference{}
}
func getGroupName(m schema.Message, refs []*schema.Reference) string {
	return getRef(m.GroupId, refs).FullName
}
func makeChannelName(m schema.Message, refs []*schema.Reference) string {
	chanName := strconv.Itoa(m.ThreadId)
	if m.DirectMessage {
		chanName = "_dm_" + chanName
	} else {
		chanName = nameHash(getGroupName(m, refs) + "_" + chanName,21,6)
	}
	chanName = strings.ToLower(chanName)
	log.Println(chanName)
	return chanName
}

func createChannel(m schema.Message, chanName string) (ch *slack.Channel, err error) {
	ch, err = api.CreateChannel(chanName)
	if err != nil {
		log.Printf("CreateChannel:%s err:%s", chanName, err)
		return
	}
	log.Printf("CreateChannel: %s", ch.Name)
	if ch.Purpose.Value, err = api.SetChannelPurpose(ch.ID, m.WebURL); err != nil {
		log.Printf("SetChannelPurpose %s,err:%s", ch.Name, err)
		return
	}
	return
}

// get Thread Parent message
func getParentRef(threadID int) (schema.Reference, error) {
	feed, err := yClient.ThreadFeed(threadID)
	if err != nil {
		return schema.Reference{}, err
	}
	for _, r := range feed.References {
		if r.Type == "message" && r.RepliedToId == 0 {
			return *r, nil
		}
	}
	return schema.Reference{}, fmt.Errorf("Can not find my parent's message. ThreadID:%d", threadID)
}
func getThreadID(m schema.Message, refs []*schema.Reference) (string, error) {
	ts, ok := threadMap[m.ThreadId]
	if ok {
		return ts, nil
	}
	ref, err := getParentRef(feed.Reference)
	if err != nil{
		return "",err
	}
	
	ch ,err := createChannel(
	if err != nil{
		return "",err
	}

	return "", nil
}
func postMsg(m schema.Message, refs []*schema.Reference) error {
	//func postMsg(m *msg) error {
	var err error
	if len(m.Body.Plain) <= 0 {
		return nil
	}
	chanName := makeChannelName(m, refs)
	ch, ok := channels[chanName]
	if !ok {
		if ch, err = createChannel(m, chanName); err != nil {
			return err
		}
		channels[ch.Name] = ch
	}
	if ch.IsArchived {
		if err = api.UnarchiveChannel(ch.ID); err != nil {
			log.Printf("UnarchiveChannel:%s err %s", ch.Name, err)
		}
		log.Printf("UnarchiveChannel: %s", ch.Name)
	}
	if !ch.IsMember {
		if ch, err = api.JoinChannel(ch.Name); err != nil {
			log.Printf("JoinChannel %s: %s", ch.Name, err)
		}
		channels[ch.Name] = ch
		log.Printf("JoinChannel: %s", ch.Name)
	}
	if ch.Purpose.Value == "" {
		if _, apierr := api.SetChannelPurpose(ch.ID, m.WebURL); apierr != nil {
			log.Printf("SetChannelPurpose %s,err:%s", ch.Name, apierr)
			return apierr
		}
	}
	sender := getRef(m.SenderId, refs)
	param := slack.PostMessageParameters{
		Username: strings.TrimSpace(nameRep.Replace(sender.FullName)),
		IconURL:  sender.MugshotURL,
	}
	if _, _, err = api.PostMessage(ch.ID, m.Body.Plain, param); err != nil {
		log.Printf("err:%s, channel:%s(%s), body:%s, param:%#v", err, ch.ID, ch.Name, m.Body.Plain, param)
	}
	log.Printf("PostMessage channel%s, user:%s", ch.Name, sender.FullName)
	//pp.Print(m)
	return nil
}

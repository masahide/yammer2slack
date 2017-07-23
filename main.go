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
	"regexp"
	"strconv"
	"strings"

	"github.com/k0kubun/pp"
	"github.com/masahide/go-yammer/cometd"
	"github.com/masahide/go-yammer/schema"
	"github.com/masahide/go-yammer/yammer"
	"github.com/nlopes/slack"
)

// Conf save file
type Conf struct {
	YammerAccessToken string
	SlackToken        string
	NetworkNameFilter string

	networkNameRe *regexp.Regexp
}

type Thread struct {
	ChannelID   string
	ChannelName string
	TS          string
}

// Cache save file
type Cache struct {
	Networks  []schema.Network
	threadMap map[int]Thread // map[thread_id]Thread
}

const (
	yammerFile = "yammer.json"
	confFile   = "yammer2slack.json"
	//slackFile     = "slack.json"
	cacheFile         = "cache.json"
	networkNameMaxLen = 10
)

var (
	conf     Conf
	channels = map[string]*slack.Channel{}
	cache    = loadCache(cacheFile) // map[thread_id]slack_ts
	nameRep  = strings.NewReplacer(
		" ", "",
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
	sClient *slack.Client
	current *schema.User
)

func init() {

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	flag.BoolVar(&debug, "debug", debug, "debug mode")
	flag.Parse()
	conf = loadConf(confFile)
	conf.networkNameRe = regexp.MustCompile(conf.NetworkNameFilter)
	yClient = yammer.New(conf.YammerAccessToken)
	yClient.DebugMode = debug
	sClient = slack.New(conf.SlackToken)
}
func getChannels() error {
	if len(channels) != 0 {
		return nil
	}
	chs, err := sClient.GetChannels(false)
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
			//receiveMessage(m.Data.Feed)
			pp.Print(m)
		}
		//saveJSON(conf, yammerFile)
	}
}

func receiveMessage(feed *schema.MessageFeed) {
	for _, mes := range feed.Messages {
		//analysis(*mes, feed.References)
		if err := postMsg(*mes, feed.References); err != nil {
			log.Print(err)
			return
		}
		//conf.InboxID = mes.Id
	}
}

func printClose(c io.Closer) {
	if err := c.Close(); err != nil {
		log.Println(err)
	}
}

/*
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
*/
func loadConf(file string) Conf {
	c := Conf{}
	f, err := os.Open(file)
	if err != nil {
		saveJSON(c, file)
		return c
	}
	defer printClose(f)
	if err = json.NewDecoder(f).Decode(&c); err != nil {
		log.Fatalln(err)
	}
	return c
}
func loadCache(filename string) Cache {
	var c Cache
	f, err := os.Open(filename)
	if err != nil {
		saveJSON(c, filename)
		return c
	}
	defer printClose(f)
	if err = json.NewDecoder(f).Decode(&c); err != nil {
		log.Fatalln(err)
	}
	return c
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

func netWorkNameShorter(name string, size int) string {
	return nameShorter(strings.TrimSpace(conf.networkNameRe.ReplaceAllString(name, "")), size)
}

func nameShorter(name string, size int) string {
	return nameHash(nameRep.Replace(name), size, 3)
}

func nameHash(name string, size, hsize int) string {
	name = nameRep.Replace(name)
	if len(name) < size {
		return name
	}
	hasher := md5.New()
	hasher.Write([]byte(name))
	h := base64.StdEncoding.EncodeToString(hasher.Sum(nil))
	if len(h) > hsize {
		log.Fatalf("len(hash)>hsize,name:%s,hsize:%s,hash:%s", name, hsize, h)
	}
	if hsize > size {
		return nameRep.Replace(h[0:hsize])
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
		chanName = nameHash(getGroupName(m, refs)+"_"+chanName, 21, 6)
	}
	chanName = strings.ToLower(chanName)
	log.Println(chanName)
	return chanName
}

func createChannel(m schema.Message, chanName string) (ch *slack.Channel, err error) {
	ch, err = sClient.CreateChannel(chanName)
	if err != nil {
		log.Printf("CreateChannel:%s err:%s", chanName, err)
		return
	}
	log.Printf("CreateChannel: %s", ch.Name)
	if ch.Purpose.Value, err = sClient.SetChannelPurpose(ch.ID, m.WebURL); err != nil {
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

func getNetwork(id int) schema.Network {
	for _, n := range cache.Networks {
		if n.ID == id {
			return n
		}
	}
	var err error
	cache.Networks, err = yClient.GetNetworks(yammer.GetNetworksOptions{})
	if err != nil {
		log.Fatalf("GetNetworks err: %s", err)
	}
	for _, n := range cache.Networks {
		if n.ID == id {
			return n
		}
	}
	log.Fatalf("not found network id: %d", id)
	return schema.Network{}

}
func getTS(m schema.Message, refs []*schema.Reference) (Thread, error) {
	thread, ok := cache.threadMap[m.ThreadId]
	if ok {
		return thread, nil
	}
	yammerParentFeed, err := getParentRef(m.ThreadId)
	if err != nil {
		return Thread{}, err
	}

	network := getNetwork(yammerParentFeed.NetworkId)
	var groupName string
	if m.DirectMessage {
		groupName = "dm"
	} else {
		groupName = getGroupName(m, refs)
	}

	thread.ChannelName = netWorkNameShorter(network.Name, 10) + "-" + nameShorter(groupName, 10)
	ch, err := createChannel(m, thread.ChannelName)
	if err != nil {
		return thread, err
	}
	sender := getRef(yammerParentFeed.SenderId, refs)
	param := slack.PostMessageParameters{
		Username: strings.TrimSpace(nameRep.Replace(sender.FullName)),
		IconURL:  sender.MugshotURL,
	}
	thread.ChannelID = ch.ID
	body := yammerParentFeed.Body.Plain + "\nsee: " + yammerParentFeed.WebURL
	if _, thread.TS, err = sClient.PostMessage(ch.ID, body, param); err != nil {
		log.Printf("err:%s, channel:%s(%s), body:%s, param:%#v", err, ch.ID, ch.Name, yammerParentFeed.Body.Plain, param)
	}
	cache.threadMap[m.ThreadId] = thread
	chJoin(thread, yammerParentFeed)
	saveJSON(cache, cacheFile)
	return thread, nil
}
func chJoin(thread Thread, parent schema.Reference) error {
	ch, err := sClient.GetChannelInfo(thread.ChannelID)
	if err != nil {
		return err
	}
	if ch.IsArchived {
		if err = sClient.UnarchiveChannel(ch.ID); err != nil {
			log.Printf("UnarchiveChannel:%s err %s", ch.Name, err)
		}
		log.Printf("UnarchiveChannel: %s", ch.Name)
	}
	if !ch.IsMember {
		if ch, err = sClient.JoinChannel(ch.Name); err != nil {
			log.Printf("JoinChannel %s: %s", ch.Name, err)
		}
		channels[ch.Name] = ch
		log.Printf("JoinChannel: %s", ch.Name)
	}
	if ch.Purpose.Value == "" {
		if _, apierr := sClient.SetChannelPurpose(ch.ID, parent.WebURL); apierr != nil {
			log.Printf("SetChannelPurpose %s,err:%s", ch.Name, apierr)
			return apierr
		}
	}
	return nil
}
func postMsg(m schema.Message, refs []*schema.Reference) error {
	//func postMsg(m *msg) error {
	var err error
	if len(m.Body.Plain) <= 0 {
		return nil
	}
	thread, err := getTS(m, refs)
	if err != nil {
		return err
	}
	sender := getRef(m.SenderId, refs)
	param := slack.PostMessageParameters{
		Username:        strings.TrimSpace(nameRep.Replace(sender.FullName)),
		IconURL:         sender.MugshotURL,
		ThreadTimestamp: thread.TS,
	}
	if _, _, err = sClient.PostMessage(thread.ChannelID, m.Body.Plain, param); err != nil {
		log.Printf("err:%s, channel:%s(%s), body:%s, param:%#v", err, thread.ChannelID, thread.ChannelName, m.Body.Plain, param)
	}
	log.Printf("PostMessage channel%s, user:%s", thread.ChannelName, sender.FullName)
	//pp.Print(m)
	return nil
}

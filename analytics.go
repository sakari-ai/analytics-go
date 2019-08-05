package analytics

import (
	"fmt"
	"io/ioutil"
	"sync"

	"bytes"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/jehiah/go-strftime"
	"github.com/segmentio/backo-go"
	"github.com/xtgo/uuid"
)

// Version of the client.
const Version = "2.1.0"

// Endpoint for the Segment API.
const Endpoint = "https://jpeg.sakari.ai"

// DefaultContext of message batches.
var DefaultContext = map[string]interface{}{
	"library": map[string]interface{}{
		"name":    "Sakari Analytics Go",
		"version": Version,
	},
}

// Backoff policy.
var Backo = backo.DefaultBacko()

// Message interface.
type message interface {
	setMessageId(string)
	setTimestamp(string)
}

// Message fields common to all.
type Message struct {
	Type      string `json:"type,omitempty"`
	MessageId string `json:"messageId,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
	SentAt    string `json:"sentAt,omitempty"`
}

// Batch message.
type Batch struct {
	Context  map[string]interface{} `json:"context,omitempty"`
	Messages []interface{}          `json:"batch"`
	Message
}

// Identify message.
type Identify struct {
	Context      map[string]interface{} `json:"context,omitempty"`
	Integrations map[string]interface{} `json:"integrations,omitempty"`
	Traits       map[string]interface{} `json:"traits,omitempty"`
	AnonymousId  string                 `json:"anonymousId,omitempty"`
	UserId       string                 `json:"userId,omitempty"`
	Message
}

// Group message.
type Group struct {
	Context      map[string]interface{} `json:"context,omitempty"`
	Integrations map[string]interface{} `json:"integrations,omitempty"`
	Traits       map[string]interface{} `json:"traits,omitempty"`
	AnonymousId  string                 `json:"anonymousId,omitempty"`
	UserId       string                 `json:"userId,omitempty"`
	GroupId      string                 `json:"groupId"`
	Message
}

// Track message.
type Track struct {
	Context      map[string]interface{} `json:"context,omitempty"`
	Integrations map[string]interface{} `json:"integrations,omitempty"`
	Properties   map[string]interface{} `json:"properties,omitempty"`
	AnonymousId  string                 `json:"anonymousId,omitempty"`
	UserId       string                 `json:"userId,omitempty"`
	Event        string                 `json:"event"`
	Message
}

// Page message.
type Page struct {
	Context      map[string]interface{} `json:"context,omitempty"`
	Integrations map[string]interface{} `json:"integrations,omitempty"`
	Properties   map[string]interface{} `json:"properties,omitempty"`
	AnonymousId  string                 `json:"anonymousId,omitempty"`
	UserId       string                 `json:"userId,omitempty"`
	Category     string                 `json:"category,omitempty"`
	Name         string                 `json:"name,omitempty"`
	Message
}

// Alias message.
type Alias struct {
	PreviousId string `json:"previousId"`
	UserId     string `json:"userId"`
	Message
}

type UserSourcing func() string

// httpClient which batches messages and flushes at the given Interval or
// when the Size limit is exceeded. Set Verbose to true to enable
// logging output.
type Client struct {
	Endpoint string
	// Interval represents the duration at which messages are flushed. It may be
	// configured only before any messages are enqueued.
	Interval time.Duration
	Size     int
	Logger   *log.Logger
	Verbose  bool

	httpClient http.Client
	key        string
	skAccount  string
	msgs       chan interface{}
	quit       chan struct{}
	shutdown   chan struct{}
	uid        func() string
	now        func() time.Time
	once       sync.Once
	wg         sync.WaitGroup
	sleep      func(int, error)
	retry      int

	user        UserSourcing
	anonymousId string

	// These synchronization primitives are used to control how many goroutines
	// are spawned by the client for uploads.
	upmtx   sync.Mutex
	upcond  sync.Cond
	upcount int
}

type Options func(c *Client)

func WithInitialUserSourcing(sourcing UserSourcing) Options {
	return func(c *Client) {
		c.user = sourcing
	}
}

func WithAnonymousId(id string) Options {
	return func(c *Client) {
		c.anonymousId = id
	}
}

func WithLogger(logger *log.Logger) Options {
	return func(c *Client) {
		c.Logger = logger
	}
}

// New client with write key.
func New(key, account string, opts ...Options) *Client {
	c := &Client{
		Endpoint:   Endpoint,
		Interval:   5 * time.Second,
		Verbose:    false,
		httpClient: *http.DefaultClient,
		key:        key,
		skAccount:  account,
		msgs:       make(chan interface{}, 100),
		quit:       make(chan struct{}),
		shutdown:   make(chan struct{}),
		now:        time.Now,
		uid:        uid,
		sleep: func(i int, err error) {
			Backo.Sleep(i)
		},
		retry: 10,
	}

	for _, apply := range opts {
		apply(c)
	}
	if c.anonymousId == "" {
		c.anonymousId = uid()
	}
	c.upcond.L = &c.upmtx
	return c
}

// Alias buffers an "alias" message.
func (c *Client) Alias(msg *Alias) error {
	if msg.UserId == "" {
		return errors.New("you must pass a 'alias.userId'")
	}

	if msg.PreviousId == "" {
		return errors.New("you must pass a 'alias.previousId'")
	}

	msg.Type = "alias"
	c.queue(msg)

	return nil
}

// Page buffers an "page" message.
func (c *Client) Page(msg *Page) error {
	if exist := msg.Context["library"]; exist == nil {
		if msg.Context == nil {
			msg.Context = DefaultContext
		} else {
			msg.Context["library"] = DefaultContext["library"]
		}
	}
	if msg.UserId == "" {
		msg.UserId = c.user()
	}
	if msg.AnonymousId == "" {
		msg.AnonymousId = c.anonymousId
	}

	msg.Type = "page"
	c.queue(msg)

	return nil
}

// Group buffers an "group" message.
func (c *Client) Group(msg *Group) error {
	if exist := msg.Context["library"]; exist == nil {
		if msg.Context == nil {
			msg.Context = DefaultContext
		} else {
			msg.Context["library"] = DefaultContext["library"]
		}
	}
	if msg.GroupId == "" {
		return errors.New("you must pass a 'groupId'")
	}

	if msg.UserId == "" {
		msg.UserId = c.user()
	}
	if msg.AnonymousId == "" {
		msg.AnonymousId = c.anonymousId
	}

	msg.Type = "group"
	c.queue(msg)

	return nil
}

// Identify buffers an "identify" message.
func (c *Client) Identify(msg *Identify) error {
	if exist := msg.Context["library"]; exist == nil {
		if msg.Context == nil {
			msg.Context = DefaultContext
		} else {
			msg.Context["library"] = DefaultContext["library"]
		}
	}
	if msg.AnonymousId == "" {
		msg.AnonymousId = c.anonymousId
	}
	if msg.UserId == "" {
		return errors.New("you must pass 'identify.userId'")
	}
	newUserId := msg.UserId
	c.user = func() string {
		return newUserId
	}

	msg.Type = "identify"
	c.queue(msg)

	return nil
}

// Track buffers an "track" message.
func (c *Client) Track(msg *Track) error {
	if exist := msg.Context["library"]; exist == nil {
		if msg.Context == nil {
			msg.Context = DefaultContext
		} else {
			msg.Context["library"] = DefaultContext["library"]
		}
	}
	if msg.Event == "" {
		return errors.New("you must pass 'track.event'")
	}

	if msg.UserId == "" {
		msg.UserId = c.user()
	}
	if msg.AnonymousId == "" {
		msg.AnonymousId = c.anonymousId
	}
	msg.Type = "track"
	c.queue(msg)

	return nil
}

func (c *Client) startLoop() {
	go c.loop()
}

// Queue message.
func (c *Client) queue(msg message) {
	c.once.Do(c.startLoop)
	msg.setMessageId(c.uid())
	msg.setTimestamp(timestamp(c.now()))
	c.msgs <- msg
}

// Close and flush metrics.
func (c *Client) Close() error {
	c.once.Do(c.startLoop)
	c.quit <- struct{}{}
	close(c.msgs)
	<-c.shutdown
	return nil
}

func (c *Client) sendAsync(msgs []interface{}) {
	c.upmtx.Lock()
	for c.upcount == 1000 {
		c.upcond.Wait()
	}
	c.upcount++
	c.upmtx.Unlock()
	c.wg.Add(1)
	go func() {
		err := c.send(msgs)
		if err != nil {
			c.logf(err.Error())
		}
		c.upmtx.Lock()
		c.upcount--
		c.upcond.Signal()
		c.upmtx.Unlock()
		c.wg.Done()
	}()
}

// Send batch request.
func (c *Client) send(msgs []interface{}) error {
	if len(msgs) == 0 {
		return nil
	}

	batch := new(Batch)
	batch.Messages = msgs
	batch.MessageId = c.uid()
	batch.SentAt = timestamp(c.now())

	b, err := json.Marshal(batch)
	if err != nil {
		return fmt.Errorf("error marshalling msgs: %s", err)
	}

	for i := 0; i < c.retry; i++ {
		if err = c.upload(b); err == nil {
			return nil
		}
		c.sleep(i, err)
	}

	return err
}

// Upload serialized batch message.
func (c *Client) upload(b []byte) error {
	url := c.Endpoint + "/v1/batch"
	req, err := http.NewRequest("POST", url, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("error creating request: %s", err)
	}

	req.Header.Add("User-Agent", "analytics-go (version: "+Version+")")
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Content-Length", string(len(b)))
	req.Header.Set("X-AuthSakari", c.key)
	req.Header.Set("X-AccountID", c.skAccount)

	res, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %s", err)
	}
	defer res.Body.Close()

	if res.StatusCode < 400 {
		c.verbose("response %s", res.Status)
		return nil
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("error reading response body: %s", err)
	}

	return fmt.Errorf("response %s: %d – %s", res.Status, res.StatusCode, string(body))
}

// Batch loop.
func (c *Client) loop() {
	var msgs []interface{}
	tick := time.NewTicker(c.Interval)

	for {
		select {
		case msg := <-c.msgs:
			c.verbose("buffer (%d/%d) %v", len(msgs), c.Size, msg)
			msgs = append(msgs, msg)
			if len(msgs) == c.Size {
				c.verbose("exceeded %d messages – flushing", c.Size)
				c.sendAsync(msgs)
				msgs = make([]interface{}, 0, c.Size)
			}
		case <-tick.C:
			if len(msgs) > 0 {
				c.verbose("interval reached - flushing %d", len(msgs))
				c.sendAsync(msgs)
				msgs = make([]interface{}, 0, c.Size)
			} else {
				c.verbose("interval reached – nothing to send")
			}
		case <-c.quit:
			tick.Stop()
			c.verbose("exit requested – draining msgs")
			// drain the msg channel.
			for msg := range c.msgs {
				c.verbose("buffer (%d/%d) %v", len(msgs), c.Size, msg)
				msgs = append(msgs, msg)
			}
			c.verbose("exit requested – flushing %d", len(msgs))
			c.sendAsync(msgs)
			c.wg.Wait()
			c.verbose("exit")
			c.shutdown <- struct{}{}
			return
		}
	}
}

// Verbose log.
func (c *Client) verbose(msg string, args ...interface{}) {
	if c.Verbose {
		if c.Logger != nil {
			c.Logger.Printf(msg, args...)
		}
	}
}

// Unconditional log.
func (c *Client) logf(msg string, args ...interface{}) {
	if c.Logger != nil {
		c.Logger.Printf(msg, args...)
	}
}

// Set message timestamp if one is not already set.
func (m *Message) setTimestamp(s string) {
	if m.Timestamp == "" {
		m.Timestamp = s
	}
}

// Set message id.
func (m *Message) setMessageId(s string) {
	if m.MessageId == "" {
		m.MessageId = s
	}
}

// Return formatted timestamp.
func timestamp(t time.Time) string {
	return strftime.Format("%Y-%m-%dT%H:%M:%S%z", t)
}

// Return uuid string.
func uid() string {
	return uuid.NewRandom().String()
}

type Properties map[string]interface{}
type Trait map[string]interface{}

func (p Properties) Set(key string, value interface{}) Properties {
	p[key] = value
	return p
}

func (tr Trait) Set(key string, value interface{}) Trait {
	tr[key] = value
	return tr
}

func NewProperties() Properties {
	return make(Properties)
}

func NewTrait() Trait {
	return make(Trait)
}

func (t *Track) SetProperties(properties Properties) {
	t.Properties = properties
}

func (p *Page) SetProperties(properties Properties) {
	p.Properties = properties
}

func (g *Identify) SetTrait(properties Trait) {
	g.Traits = properties
}

func (g *Group) SetTrait(properties Trait) {
	g.Traits = properties
}

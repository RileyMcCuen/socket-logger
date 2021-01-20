package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/gorilla/websocket"
	"log"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"sync"
)

//// CL args

// Address to listen on
var faddr = flag.String("faddr", "localhost:9000", "http service address")

// Whether to open a page in the browser or not
var fopen = flag.Bool("open", false, "whether to open a new page in the browser")

// Whether to log anything to the local console or not
var flog = flag.Bool("log", false, "whether to log to the local console or not")

// Which directory to server static content from
var fdir = flag.String("dir", "./static/", "what directory to run static file server from, if empty static files are not served")

// The name of the homepage in the specified directory fdir, only matters if fdir is not ""
var fhome = flag.String("home", "index.html", "the home page that will be opened automatically")

// What level of messages to propagate to the receiving applications.
var flevel = new(Level)

// Number of elements to retain in history, -1 is all elements TODO: use this var and send array of messages over socket instead of 1
// var fhist flag.Int("hist", 0, "number of logs to keep in history, -1 will retain all elements, when a new connection is made all history up to that point will be sent over the socket")

//// Utility

const clearOnStartMessage = "clears"
const clearOnFinishMessage = "clearf"

// Used to open browser when the server is run. Got the code form this url:
// https://stackoverflow.com/questions/39320371/how-start-web-server-to-open-page-in-browser-in-golang
func open(url string) {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
	}
	args = append(args, url)
	_ = exec.Command(cmd, args...).Start()
}

// Configures logger based on flog flag
func configureLogger() {
	if *flog {
		log.SetFlags(log.LstdFlags)
	} else {
		log.SetFlags(0)
	}
}

// Configure static file server based on fdir and fhome flags
func configureStaticFileServer() {
	if *fdir != "" {
		fs := http.FileServer(http.Dir(*fdir))
		if *fopen {
			go open("http://" + *faddr + "/static/" + *fhome)
		}
		http.Handle("/static/", http.StripPrefix("/static", fs))
	}
}

//// Global Types

// Level represents the importance of the log message.
type Level int

func (l *Level) String() string {
	return fmt.Sprintf("%d", *l)
}

func (l *Level) Set(input string) error {
	convertedInput, err := strconv.Atoi(input)
	if err != nil {
		return err
	}
	*l = Level(convertedInput)
	return nil
}

const (
	clearOnStart = -2
	clearOnFinish = -1
	debug Level = iota
	info
	warn
	er
)

// A Message is what is expected to be sent from sending applications and is what
// is sent to receiving applications.
type Message struct {
	Time int64 `json:"time"`
	Content string `json:"content"`
	LoggerName string `json:"logger_name"`
	FileName string `json:"file_name"` // empty string if not available
	LineNum int `json:"line_num"` // -1 if not available
	ColumnNum int `json:"column_num"` // -1 if not available
	Level Level `json:"level"` // level of log importance
}

// Tries to create a new message from the input data, returns an error if the data cannot be unmarshalled
func NewMessage(message []byte) (*Message, error) {
	msg := &Message{
		Time:     0,
		Content:  "",
		FileName: "",
		LineNum:  -1,
		ColumnNum: -1,
		Level:      0,
	}
	if err := json.Unmarshal(message, msg); err != nil {
		return nil, err
	}
	if msg.Level < debug || msg.Level > er {
		msg.Level = er
	}
	return msg, nil
}

// Used for receiving and broadcasting messages
type Fan struct {
	in chan *Message
	counter int
	out map[int]*websocket.Conn
	sync.Mutex
}

// Adds a connection to the global fan
func (f *Fan) addConn(c *websocket.Conn) int {
	f.Lock()
	defer f.Unlock()

	f.counter += 1 //TODO: make sure counter is not already in use
	log.Printf("Opened conn on number: %d\n", f.counter)
	f.out[f.counter] = c
	return f.counter
}

// Removes a connection from the global fan
func (f *Fan) removeConn(num int) {
	f.Lock()
	defer f.Unlock()

	log.Printf("Removed conn on number: %d\n", f.counter)
	c := f.out[num]
	if c != nil {
		_ = c.Close()
	}
	delete(f.out, num)
}

// Listen for all incoming messages on in and broadcast them
func (f *Fan) listen() {
	var message *Message
	for {
		message = <- f.in
		f.broadcast(message, &bcontext{})
	}
}

// Context for queueing operation
type qcontext struct {}

// Context for broadcasting operation
type bcontext struct {}

// Tries to unmarshal a new message that has been received then puts it in
// queues it in the broadcasting channel
func (f *Fan) queue(message []byte, ctx *qcontext) error {
	msg := &Message{}
	if len(message) == len(clearOnStartMessage) && string(message) == clearOnStartMessage {
		msg.Level = clearOnStart
		f.in <- msg
	}
	if len(message) == len(clearOnFinishMessage) && string(message) == clearOnFinishMessage {
		msg.Level = clearOnFinish
		f.in <- msg
	} else {
		msg, err := NewMessage(message)
		if err != nil {
			return err
		}
		if msg.Level >= *flevel {
			f.in <- msg
		} else {
			log.Println("did not queue message due to log level")
		}
	}
	return nil
}

// Broadcasts a message to all registered receivers, if an error occurs the
// offending connection is removed from the Fan.
func (f *Fan) broadcast(message *Message, ctx *bcontext) {
	f.Lock()
	defer f.Unlock()
	for num, c := range f.out {
		if err := c.WriteJSON(message); err != nil {
			f.removeConn(num)
		}
	}
}

//// Globals

// Used to upgrade connections to use websockets
var upgrader = websocket.Upgrader{} // use default options

// Used for fanning out logs
var fan = &Fan{
	in:      make(chan *Message, 0),
	out:     make(map[int]*websocket.Conn, 0),
	counter: 0,
	Mutex:   sync.Mutex{},
}

//// Handlers

// Used to set up a websocket to receive messages.
func receive(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	num := fan.addConn(c)
	defer fan.removeConn(num)
	for {
		_, message, err := c.ReadMessage()
		if err != nil {
			log.Println("read:", err)
			break
		}
		log.Printf("recv: %s", message)
		if string(message) == "close" {
			log.Println("closed")
			break
		}
	}

}

// Used tp set up a websocket to send messages
func send(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer func() { _ = c.Close() }()
	for {
		_, message, err := c.ReadMessage()
		if err != nil {
			log.Println("read:", err)
			break
		}
		log.Printf("recv: %s", message)
		if err = fan.queue(message, &qcontext{}); err != nil {
			_ = c.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("{\"error\":\"Message did not abide by the proper format.\", \"message\":\"%s\"}", string(message))))
		}
	}
}

//// Main

// Entrypoint for the server
func main() {
	// Get command line flags
	flag.Var(flevel, "level", "what level of messages to send through the fan, range:[0,3], lower is less restrictive")
	flag.Parse()

	// Configure logger depending on command line flags
	configureLogger()

	// Handle all messages that come into the fan
	go fan.listen()

	// Setup websocket routes
	http.HandleFunc("/rec", receive)
	http.HandleFunc("/send", send)

	// Setup static file server
	configureStaticFileServer()

	// Fire up the web server and listen
	log.Fatal(http.ListenAndServe(*faddr, nil))
}

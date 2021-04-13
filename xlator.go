package main

import (
	"fmt"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"regexp"
	"strconv"
)

var debugLogging bool = true

//////////
// Structs
//////////

// A concurrency primitive
type Command struct {
	Type string
	Ticket *Ticket
	TicketId int64
	ReplyChannel chan []byte
}
// Represents a single request for translation and the translator's response.
type Ticket struct {
	Id 				int64
	MessageType 	string
	Message 		string
	ResponseType 	string
	Response 		string
}
// Represents the web server
type Server struct {
	Commands chan<- Command
}
// Handles incoming add requests
func (s *Server) add(response http.ResponseWriter, request *http.Request) {
	if debugLogging { 
		fmt.Println("Received "+request.RequestURI+" from "+request.RemoteAddr) 
	}

	if request.Method == "POST" {
		// Add a new ticket to the collection
		body, error := ioutil.ReadAll(request.Body)
		if error != nil {
			fmt.Println("Io error: ", error)
		}
		
		ticket, jerr := byteToTicket(body)
		if jerr != nil {
			BadRequest(response, request)
		}

		// Tell the board manager to add the ticket
		replyChannel := make(chan []byte) 
		s.Commands <- Command{"add", &ticket, ticket.Id, replyChannel}
		data := <- replyChannel

		// Expect to get the ticket back over the channel
		if len(data) == 0 {
			BadRequest(response, request)
		} else {
			fmt.Println("Added ticket ", ticket.Id)
		}
	}
}
// Handles incoming list requests
func (Server) list(response http.ResponseWriter, request *http.Request) {
	if debugLogging { 
		fmt.Println("Received "+request.RequestURI+" from "+request.RemoteAddr) 
	}

	if request.Method == "GET" {
		// communicate with the board manager to return the list of tickets
		// 200 OK if successful, 400 Bad request otherwise
	} else if request.Method == "POST" {
		// serialize the incoming data
		// update the list item with the specified id
		// 200 OK if successful, 400 Bad request otherwise
	}
}
// Handles incoming get requests
func (s *Server) get(response http.ResponseWriter, request *http.Request) {
	if debugLogging { 
		fmt.Println("Received "+request.RequestURI+" from "+request.RemoteAddr) 
	}

	if request.Method == "GET" {
		incomingId := request.URL.Query().Get("id")
		id, converr := strconv.ParseInt(incomingId, 10, 64)
		if converr != nil {
			fmt.Println("Error parsing id:", converr)
		}

		replyChannel := make(chan []byte) 
		s.Commands <- Command{"get", &Ticket{}, id, replyChannel}
		data := <- replyChannel
		fmt.Println(string(data))
		// Expect to get the ticket back over the channel
		if len(data) == 0 {
			BadRequest(response, request)
		} else {
			response.Write(data)
		}
	}
}

//////////
// Helpers
//////////

// Converts an array of bytes to a Ticket
func byteToTicket(rawBytes []byte) (Ticket, error) {
	var t Ticket
	jsonerr := json.Unmarshal(rawBytes, &t)
	if jsonerr != nil {
		fmt.Printf("Error: %s\n", jsonerr)
	}
	return t, jsonerr
}

// Converts json to an array of bytes
func ticketToByte(ticket *Ticket) ([]byte, error) {
	b, jsonerr := json.Marshal(ticket)
	if jsonerr != nil {
		fmt.Println("Error encoding ticket:", jsonerr)
	}
	return b, jsonerr
}

// This is where the magic happens. Spins up a goroutine which constantly processes jobs from the handlers
func startBoardManager() chan<- Command {
	Tickets := make(map[int64]*Ticket)
	Commands := make(chan Command)
	
	go func () {
		if debugLogging { fmt.Println("BoardManager: Started") }
		
		// Read from the command channel forever
		for cmd := range Commands {
			switch cmd.Type {
			case "add": // Add the incoming ticket to the map of tickets. The handler
				// expects a non-empty byte array from the response channel to indicate success.
				if debugLogging { fmt.Println("BoardManager: Add") }
				Tickets[cmd.Ticket.Id] = cmd.Ticket
				cmd.ReplyChannel <- []byte{ 0 }
				if debugLogging { fmt.Println("BoardManager: Tickets =", Tickets) }				
			case "list": 
				// TODO: Return a list of tickets
				if debugLogging { fmt.Println("BoardManager: List") }
			case "get": 
				// TODO: Return a specific ticket. Move heavy processing into the event handler. Use a channel to 
				// pass the ID to the board manager then pass the ticket pointer back through the response channel.
				// then Marshal ticket in the event handler and send it in the response. 
				if debugLogging { fmt.Println("BoardManager: Get") }
				if Val, ok := Tickets[cmd.TicketId]; ok {
					jsonBytes, jerr := ticketToByte(Val)
					if jerr != nil {
						cmd.ReplyChannel <- []byte{}
					} else {
						cmd.ReplyChannel <- jsonBytes
					}
				} else {
					fmt.Println("Error getting ticket:", ok)
					cmd.ReplyChannel <- []byte{}
				}
			case "modify":
				// TODO: Write the updated ticket to the map of tickets.
			default:
				fmt.Println("BoardManager: unknown command", cmd.Type)
			}
		}
	}()
	return Commands
}

///////////
// net/http Methods
///////////

// A quick and easy method to respond with a BadRequest  
func BadRequest(w http.ResponseWriter, r *http.Request) { 
	http.Error(w, "400 Bad request", http.StatusBadRequest) 
}

// A wrapper for net/http handler methods. Ensures unexpected handlers won't be created.
func createHandler(thisHandler func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	var validPath = regexp.MustCompile("^/(add|list|get|([0-9]+))$")
	return func (response http.ResponseWriter, request *http.Request) {
		m := validPath.FindStringSubmatch(request.URL.Path)
		if m == nil {
			BadRequest(response, request)
			return
		}
		thisHandler(response, request)
	}
}

///////////
// main
///////////

func main() {
	// var jsonBlob = []byte(`{
	// 	"id": 12345678, 
	// 	"messageType": "binary", 
	// 	"message": "1AD3F5DC341EE61ABDF9789B32FEDCBA"
	// }`)
	cmdchan := startBoardManager()
	var server Server = Server{cmdchan}
	
	http.HandleFunc("/add", createHandler(server.add))
	http.HandleFunc("/list", createHandler(server.list))
	http.HandleFunc("/get", createHandler(server.get))
	
	port := "8000"
	error := http.ListenAndServe("localhost:"+port,nil)
	if error != nil {
		fmt.Printf("Error: %s\n", error)
	}
}
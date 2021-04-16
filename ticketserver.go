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
var extraLogging bool = false

//////////
// Structs
//////////

// A concurrency primitive to pass data to the board manager
type Command struct {
	Type string
	Ticket *Ticket
	TicketId int64
	ReplyChannel chan CommandResponse
}
// Another primitive for passing data from the board manager
type CommandResponse struct {
	Bytes []byte
	Ticket *Ticket
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
			return
		}

		// Tell the board manager to add the ticket
		replyChannel := make(chan CommandResponse) 
		s.Commands <- Command{"add", &ticket, ticket.Id, replyChannel}
		cmdResponse := <- replyChannel

		// Expect to get the ticket back over the channel
		if len(cmdResponse.Bytes) == 0 {
			BadRequest(response, request)
			return
		} else {
			fmt.Println("Added ticket ", ticket.Id)
		}
	}
}
// Handles incoming list requests
func (s *Server) list(response http.ResponseWriter, request *http.Request) {
	if debugLogging { 
		fmt.Println("Received "+request.RequestURI+" from "+request.RemoteAddr) 
	}

	if request.Method == "GET" {
		// communicate with the board manager to return the list of tickets
		// TODO: Add a limit so only a certain number of tickets will be returned
		if len(string(request.URL.Query().Get("id"))) != 0 {
			BadRequest(response, request)
			return
		} 

		// Tell the board manager to get the list of tickets
		replyChannel := make(chan CommandResponse) 
		s.Commands <- Command{"list", &Ticket{}, 0, replyChannel}
		
		var dataAccum []byte
		for cmdResponse := range replyChannel {
			if cmdResponse.Ticket != nil {
				ticketBytes, serr := ticketToByte(cmdResponse.Ticket)
				if serr != nil {
					fmt.Println("Error deserializing ticket: ", serr)
					BadRequest(response, request)
					return
				}
				dataAccum = append(dataAccum, ticketBytes...)
				if extraLogging == true { fmt.Println("Accumulating... ", string(dataAccum)) }
			} else {
				break
			}
		}
		if debugLogging == true { fmt.Printf("Sending Response: %s\n", string(dataAccum))}
		response.Write(dataAccum)

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

		replyChannel := make(chan CommandResponse)
		s.Commands <- Command{"get", &Ticket{}, id, replyChannel}
		cmdResponse := <- replyChannel

		if len(cmdResponse.Bytes) == 0 {
			BadRequest(response, request)
			return
		} else {
			response.Write(cmdResponse.Bytes)
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
		
		// Read from the command channel forever. 
		// Any delays incurred here will impact every goroutine waiting on a reply from the board manager
		for cmd := range Commands {
			switch cmd.Type {
			case "add": // Add the incoming ticket to the map of tickets. The handler
				// expects a non-empty byte array from the response channel to indicate success.
				if debugLogging { fmt.Println("BoardManager: Add") }
				Tickets[cmd.Ticket.Id] = cmd.Ticket
				cmd.ReplyChannel <- CommandResponse{Bytes: []byte{ 0 }}
				if debugLogging { fmt.Println("BoardManager: Tickets =", Tickets) }				
			case "list": 
				// TODO: Return a list of tickets
				if debugLogging { fmt.Println("BoardManager: List") }

				for _, t := range Tickets {
					cmd.ReplyChannel <- CommandResponse{Ticket: t}
				}

				cmd.ReplyChannel <- CommandResponse{}
				fmt.Println("BoardManager: Done Listing")

			case "get": 
				if debugLogging { fmt.Println("BoardManager: Get") }
				// TODO: Move the heavy processing out of here
				if Val, ok := Tickets[cmd.TicketId]; ok {
					jsonBytes, jerr := ticketToByte(Val) // This needs to be tested for speed
					if jerr != nil {
						cmd.ReplyChannel <- CommandResponse{Bytes: []byte{}}
					} else {
						cmd.ReplyChannel <- CommandResponse{Bytes: jsonBytes}
					}
				} else {
					fmt.Println("Error getting ticket:", ok)
					cmd.ReplyChannel <- CommandResponse{Bytes: []byte{}}
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
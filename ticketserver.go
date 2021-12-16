package main

import (
	"fmt"
	"flag"
	"os"
	"time"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

/////////////////////////////
//         Structs         //
/////////////////////////////

// Concurrency primitive
type Command struct {
	Type string
	Ticket *Ticket
	TicketId int64 // Index into the map
	ReplyChannel chan CommandResponse
}

// Response primitive
type CommandResponse struct {
	Bytes []byte
	Ticket *Ticket
}

// The control structure
type Ticket struct {
	Id  			int64
	MessageType 	string
	Message 		string
	ResponseType 	string
	Response 		string
	Complete		bool
}

/////////////////////////////
//         Server          //
/////////////////////////////

type Server struct {
	Commands chan<- Command
	verbose bool
}

/////////////////////////////
//        Handlers         //
/////////////////////////////

func (s *Server) add(response http.ResponseWriter, request *http.Request) {
	if s.verbose { 
		fmt.Println("Receive " + request.RemoteAddr + " ADD") 
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
			fmt.Println("Done [" + strconv.FormatInt(ticket.Id, 10) + "]")
		}
	}
}

func (s *Server) list(response http.ResponseWriter, request *http.Request) {
	if request.Method == "GET" {
		if s.verbose { 
			fmt.Println("Receive " + request.RemoteAddr + " LIST") 
		}

		// communicate with the board manager to return the list of tickets
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
			} else {
				break
			}
		}
		if s.verbose == true { fmt.Printf("Sending Response: %s\n", string(dataAccum))}
		response.Write(dataAccum)

	} else if request.Method == "POST" {
		if s.verbose { 
			fmt.Println("Receive " + request.RemoteAddr + " MODIFY") 
		}

		// serialize the incoming data
		// update the list item with the specified id
		// 200 OK if successful, 400 Bad request otherwise
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

		// Tell the board manager to modify the ticket
		replyChannel := make(chan CommandResponse) 
		s.Commands <- Command{"modify", &ticket, ticket.Id, replyChannel}
		cmdResponse := <- replyChannel

		// Expect to get the ticket back over the channel
		if len(cmdResponse.Bytes) == 0 {
			BadRequest(response, request)
			return
		} else {
			fmt.Println("Done [" + strconv.FormatInt(ticket.Id, 10) + "]")
		}

	}
}

func (s *Server) get(response http.ResponseWriter, request *http.Request) {
	if s.verbose { 
		fmt.Println("Received "+request.RequestURI+" from "+request.RemoteAddr) 
	}

	if request.Method == "GET" {
		incomingOp := request.URL.Query().Get("op")
		if (len(incomingOp) > 0) {
			s.Commands <- Command{incomingOp, &Ticket{}, 0, nil}
			return;
		}

		incomingId := request.URL.Query().Get("id")
		id, converr := strconv.ParseInt(incomingId, 10, 64)
		if converr != nil {
			fmt.Println("Error parsing id:", converr)
		}

		replyChannel := make(chan CommandResponse)
		s.Commands <- Command{"get", &Ticket{}, id, replyChannel}
		cmdResponse := <- replyChannel

		if cmdResponse.Ticket.Id == 0 {
			BadRequest(response, request)
			return
		} else {
			jsonBytes, jerr := ticketToByte(cmdResponse.Ticket)
			if jerr != nil {
				fmt.Println("Error marshalling JSON: ", jerr)
			} else {
				response.Write(jsonBytes)
			}
		}
	}
}

/////////////////////////////
//        Methods          //
/////////////////////////////

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

// Write contents of the map to files then delete the entries
func exportTickets(tickets map[int64]*Ticket) {
	currentTime := time.Now();
	exportFolder := "tickets_" + currentTime.Format("2006_01_02_15_04_05")
	os.Mkdir(exportFolder, 0755);

	for _, t := range tickets {
		fmt.Printf("Exporting [%d]\n", t.Id)
		dat := []byte("messageType=" + t.MessageType + "\n" +
					  "message=" + t.Message + "\n" +
					  "responseType=" + t.ResponseType + "\n" +
					  "response=" + t.Response + "\n" +
					  "complete=" + strconv.FormatBool(t.Complete) + "\n");
		err := os.WriteFile(exportFolder +  "/ticket#" + strconv.FormatInt(t.Id, 16), dat, 0644);
		if (err != nil) {
			fmt.Printf("Error: " + err.Error() + "\n");
		}
	}

	for i := range tickets {
		delete(tickets, i)
	}
}

// Read contents of files on disk into the map
func importTickets(tickets map[int64]*Ticket, workingDir string) {
	files, err := ioutil.ReadDir(workingDir)
	if (err != nil) {
		fmt.Println("Error opening import directory: " + err.Error())
		return
	}

	for _, f := range files {
		bytesRead, _ := ioutil.ReadFile(workingDir + f.Name())
		fileContent := string(bytesRead)
		lines := strings.Split(fileContent, "\n")

		var t Ticket
		idStr := strings.Split(f.Name(), "#")[1]
		ticketId, _ := strconv.ParseInt(idStr, 16, 64)
		t.Id = ticketId
		
		for _, l := range lines {
			tokens := strings.Split(l, "=")
			switch (tokens[0]) {
			case "messageType":
				t.MessageType = tokens[1]
				break
			case "message":
				t.Message = tokens[1]
				break
			case "responseType":
				t.ResponseType = tokens[1]
				break
			case "response":
				t.Response = tokens[1]
				break
			case "complete":
				result, _ := strconv.ParseBool(tokens[1])
				t.Complete = result
			}
		}
		tickets[t.Id] = &t
		fmt.Println("Added Id=" + strconv.FormatInt(ticketId, 10))
	}
}

// Modify the entry t with the values specified
func modifyTicket(t *Ticket, messageType string, message string, responseType string, response string, complete bool) {
	if (t != nil) {
		t.MessageType = messageType;
		t.Message = message;
		t.ResponseType = responseType;
		t.Response = response;
		t.Complete = complete;
	} else {
		fmt.Printf("Error: Attempted modification on a nil ticket.");
	}
}

/////////////////////////////
//         Worker          //
/////////////////////////////

// Starts a goroutine which constantly processes jobs from the command channel. 
func startBoardManager(importDir string) chan<- Command {

	Tickets := make(map[int64]*Ticket)
	Commands := make(chan Command)
	
	if (len(importDir) > 0 ) { importTickets(Tickets, importDir) }

	go func () {

		// fmt.Println("BoardManager: Started")
		
		// Receive from the command channel forever. Handler goroutines will send commands via the 
		// Command channel. When you read from a channel it waits until a value is there. When handlers send
		// a value on the command channel the sender blocks until the receiver is ready to receive it.
		for cmd := range Commands {

			switch cmd.Type {

			// Add the incoming ticket to the map
			case "add": 
				if Key, found := Tickets[cmd.Ticket.Id]; found {
					cmd.ReplyChannel <- CommandResponse{Ticket: Key} // Error
				} else {
					// fmt.Printf("BoardManager: Adding new ticket.\n")
					Tickets[cmd.Ticket.Id] = cmd.Ticket
					cmd.ReplyChannel <- CommandResponse{Bytes: []byte{ 0 }}
				}
				// fmt.Println("BoardManager: Tickets =", Tickets)

			// Respond to the handler with a list of tickets
			// TODO: Add a limit so only a certain number of tickets will be returned and
			// the channel won't be blocked for others. This will depend on the use-case.
			case "list": 
				// fmt.Println("BoardManager: List")
				for _, t := range Tickets {
					cmd.ReplyChannel <- CommandResponse{Ticket: t}
				}
				// Once finished send an empty response
				cmd.ReplyChannel <- CommandResponse{}

			// Respond to the handler with a specific ticket
			case "get": 
				// fmt.Println("BoardManager: Get")
				if Val, ok := Tickets[cmd.TicketId]; ok {
					// fmt.Println("BoardManager: Returning ticket ", cmd.TicketId)
					cmd.ReplyChannel <- CommandResponse{Ticket: Val}
				} else {
					// fmt.Printf("BoardManager: Ticket %d not found.\n", cmd.TicketId)
					cmd.ReplyChannel <- CommandResponse{Ticket: &Ticket{}}
				}
			
			// Modify a specific ticket
			case "modify":
				if Key, found := Tickets[cmd.Ticket.Id]; found {
					// fmt.Printf("BoardManager: Updating ticket %d.\n", Key.Id)
					// Spinning up a go routine is **PROBABLY** a bad idea. The whole point of using channels is for 
					// synchronizing access to the control structure. A go routine here would result in a data race
					// go modifyTicket(ticket, cmd.Ticket.MessageType, cmd.Ticket.Message, cmd.Ticket.ResponseType, cmd.Ticket.Response, cmd.Ticket.Complete);
					modifyTicket(Key, cmd.Ticket.MessageType, cmd.Ticket.Message, cmd.Ticket.ResponseType, cmd.Ticket.Response, cmd.Ticket.Complete);
					// fmt.Println("BoardManager: Modify done.\n", ticket)
					cmd.ReplyChannel <- CommandResponse{Bytes: []byte{ 0 }}
				} else {
					// fmt.Printf("BoardManager: Ticket does not exist.\n")
					cmd.ReplyChannel <- CommandResponse{Bytes: []byte{}} // Error
				}
				// fmt.Println("BoardManager: Tickets =", Tickets)

			// Page out the tickets and clear the control structure
			case "export":
				// fmt.Printf("BoardManager: Exporting %d tickets...\n", len(Tickets))
				exportTickets(Tickets);
			
			// Log statistics
			case "stat":
				fmt.Printf("Tickets=%d\n", len(Tickets))

			// Debugging
			default:
				// fmt.Println("BoardManager: Listing tickets...")
				// for _, t := range Tickets {
				// 	fmt.Println("Ticket: ", t)
				// }
			}
		}
	}() // DO NOT REMOVE

	return Commands
}

/////////////////////////////
//        Net/HTTP         //
/////////////////////////////

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

/////////////////////////////
//          main           //
/////////////////////////////

func main() {
	portVar    := flag.String("port", "8000", "a string")
	importVar  := flag.String("import", "", "a string")
	verboseVar := flag.Bool("verbose", false, "a bool")
	flag.Parse()

	cmdchan := startBoardManager(*importVar)
	var server Server = Server {Commands: cmdchan, verbose: *verboseVar}
	http.HandleFunc("/add", createHandler(server.add))
	http.HandleFunc("/list", createHandler(server.list))
	http.HandleFunc("/get", createHandler(server.get))
	
	error := http.ListenAndServe("localhost:" + *portVar, nil)
	if error != nil {
		fmt.Printf("Error: %s\n", error)
	}
}
# go-ticket
A simple ticket system in Go.

## Description
A command-line application that listens on a designated port for ticket management operations. The server supports jwt-based authentication and allows clients to add, modify, and list tickets over HTTP. A "ticket" is just a data structure that stores a few different pieces of information:

- Unique id
- Message
- Message type
- Response
- Response type
- Flag indicating completion

All tickets are stored in memory in a map based on the id. The operations of the server are intentionally generic and act as a skeleton for abstraction. 

## Running the Server

The server may be launched by running the executable with the desired combination of flags. The below command will display all available flags.

`.\ticketserver.exe -h` 

### Add

To add a ticket to the server, issue a `POST` request to the `/add` endpoint. In the PostData, specify the JSON encoded ticket.

`{"id":23412346,"messageType":"binary","message":"1AD3F5DC341EE61ABDF9789B32FEDCBA"}`

### Get

To get a single ticket from the server, issue a `GET` request to the `/get` endpoint. In the URL query parameters, specify the id of the ticket.

`/get?id=12345`

### List

To get a list of all tickets, issue a `GET` request to the `/list` endpoint. The server will return the entire list of tickets in the response.

### Update

To update a ticket, issue a `POST` request to the `/list` endpoint. In the PostData, specify the JSON encoded modified ticket. The id must match an existing ticket or the server will return an error.

### Export

To export all tickets to disk, issue a `GET` request to the `/get` endpoint. In the URL query parameters, specify the `export` operation.

`/get?op=export`
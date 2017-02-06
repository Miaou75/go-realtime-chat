package main

import (
	"log"
	"net/http"
	"fmt"
	"database/sql"
	_ "github.com/lib/pq"
	"github.com/abbot/go-http-auth"
	"github.com/gorilla/websocket"
)

const (
  host     = "localhost"
  port     = 5432
  user     = "postgres"
  password = "Miaou"
  dbname   = "chatdb"
)

var clients = make(map[*websocket.Conn]bool) // connected clients
var broadcast = make(chan Message)           // broadcast channel

var psqlInfo string
var db *sql.DB

func init(){
  var err error
  psqlInfo = fmt.Sprintf("host=%s port=%d user=%s "+
      "password=%s dbname=%s sslmode=disable",
      host, port, user, password, dbname)
  db, err = sql.Open("postgres", psqlInfo)
  checkErr(err)
  err = db.Ping()
  checkErr (err)
  fmt.Println("Successfully connected to database!")
}

func checkErr(err error) {
  if err != nil {
      panic(err)
  }
}

// Configure the upgrader
var upgrader = websocket.Upgrader{
  CheckOrigin: func(r *http.Request) bool {
  return true
  },
}

func Secret(db *sql.DB, user, realm string) string {
  var (
      username string
      password string
  )
  err := db.QueryRow("select name, password from users where name = $1", user).Scan(&username, &password)
  if err == sql.ErrNoRows {
  fmt.Println("No row")
  return ""
  }
  if err != nil {
      log.Fatal(err)
  }
  return password
}

// Message message object
type Message struct {
  /*Email    string `json:"email"`*/
  Username string `json:"username"`
  Message  string `json:"message"`
}

func main() {
  defer db.Close()
  authenticator := auth.NewBasicAuthenticator("Ptdrouze Chat", func(user, realm string) string {
	return Secret(db, user, realm)
  })

  http.HandleFunc("/", authenticator.Wrap(func(res http.ResponseWriter, req *auth.AuthenticatedRequest) {
  http.FileServer(http.Dir("../public")).ServeHTTP(res, &req.Request)
  }))

	// Configure websocket route
	http.HandleFunc("/ws", handleConnections)

	// Start listening for incoming chat messages
	go handleMessages()

	// Start the server on localhost port 8000 and log any errors
	log.Println("http server started on :8000")
	err := http.ListenAndServe(":8000", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

func handleConnections(w http.ResponseWriter, r *http.Request) {
	// Upgrade initial GET request to a websocket
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("error %v", err)
		return
	}

	// Make sure we close the connection when the function returns
	defer ws.Close()

	// Register our new client
	clients[ws] = true

	for {
		var msg Message
		// Read in a new message as JSON and map it to a Message object
		err := ws.ReadJSON(&msg)
		if err != nil {
			log.Printf("error: %v", err)
			delete(clients, ws)
			break
		}
		// Send the newly received message to the broadcast channel
		broadcast <- msg
	}
}

func handleMessages() {
	for {
		// Grab the next message from the broadcast channel
		msg := <-broadcast
		// Send it out to every client that is currently connected
		for client := range clients {
			err := client.WriteJSON(msg)
			if err != nil {
				log.Printf("error: %v", err)
				client.Close()
				delete(clients, client)
			}
		}
	}
}

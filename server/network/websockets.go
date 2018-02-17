package network

import (
	"github.com/gorilla/websocket"
	"github.com/osum4est/awesome-sms-server/database"
	"net/http"
)

type websocketServer struct {
	db       *database.DB
	upgrader *websocket.Upgrader
}

func NewWebSocketServer(db *database.DB) *websocketServer {
	return &websocketServer{db, &websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		}}}
}

func (server *websocketServer) newConnection(w http.ResponseWriter, r *http.Request) {
	// Upgrade initial GET request to a websocket
	ws, err := server.upgrader.Upgrade(w, r, nil)
	if err != nil {
		panic(err)
	}

	// Make sure we close the connection when the function returns
	defer ws.Close()

	for {
		message := make([]interface{}, 0)
		err := ws.ReadJSON(&message)
		if err != nil {
			panic(err) // TODO: Send bad request error
		}

		json := message[1].(map[string]interface{})
		switch message[0] {
		case "get_new_messages":
			server.getNewMessages(json, ws)
		case "get_threads":
			server.getThreads(json, ws)
		}
	}
}

func (server *websocketServer) getNewMessages(json map[string]interface{}, ws *websocket.Conn) {
	lastDateReceived := int64(json["lastDateReceived"].(float64))
	amount := int(json["amount"].(float64))
	messages := server.db.MessageTable.GetNewMessages(lastDateReceived, amount)
	ws.WriteJSON([2]interface{}{"new_messages", messages})
}

func (server *websocketServer) getThreads(json map[string]interface{}, ws *websocket.Conn) {
	// TODO: Rename because amount is misleading
	amount := int(json["amount"].(float64))
	messages := server.db.MessageTable.GetThreads(amount)
	ws.WriteJSON([2]interface{}{"new_messages", messages})
}

package main

import (
	"fmt"
	"log"
	"net/http"
	"webrtc-sip-server/internal/signaling"
	"webrtc-sip-server/internal/sip"
)

func main() {
	sipServer := sip.NewServer()
	go sipServer.Start(":5060")

	wsHandler := signaling.NewWebSocketHandler(sipServer)

	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/", fs)
	http.HandleFunc("/ws", wsHandler.HandleConnection)

	fmt.Println("Server starting...")
	fmt.Println("HTTP server on :8080")
	fmt.Println("SIP server on :5060 (UDP)")
	fmt.Println("Open http://localhost:8080 in browser")

	log.Fatal(http.ListenAndServe(":8080", nil))
}

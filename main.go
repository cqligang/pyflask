package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

//go:embed static/*
var staticFiles embed.FS

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type SignalingMessage struct {
	Type      string                   `json:"type"`
	SDP       string                   `json:"sdp,omitempty"`
	Candidate *webrtc.ICECandidateInit `json:"candidate,omitempty"`
	Data      string                   `json:"data,omitempty"`
}

func main() {
	subFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatal(err)
	}

	http.Handle("/", http.FileServer(http.FS(subFS)))
	http.HandleFunc("/ws", handleWebSocket)

	fmt.Println("WebRTC Go Client")
	fmt.Println("================")
	fmt.Println("Open http://localhost:8080 in your browser")
	fmt.Println("Server listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}
	defer conn.Close()

	log.Println("New WebSocket connection")

	peerConnection, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	})
	if err != nil {
		log.Println("Create PeerConnection error:", err)
		return
	}
	defer peerConnection.Close()

	dataChannel, err := peerConnection.CreateDataChannel("chat", nil)
	if err != nil {
		log.Println("Create DataChannel error:", err)
		return
	}

	dataChannel.OnOpen(func() {
		log.Println("DataChannel opened")
		dataChannel.SendText("Hello from Go WebRTC client!")
	})

	dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
		log.Printf("Received from DataChannel: %s", string(msg.Data))
		if msg.IsString {
			reply := fmt.Sprintf("Go received: %s", string(msg.Data))
			dataChannel.SendText(reply)
		}
	})

	dataChannel.OnClose(func() {
		log.Println("DataChannel closed")
	})

	peerConnection.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		candidate := c.ToJSON()
		msg := SignalingMessage{
			Type:      "ice",
			Candidate: &candidate,
		}
		conn.WriteJSON(msg)
	})

	peerConnection.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		log.Printf("Peer Connection State: %s", s.String())
		if s == webrtc.PeerConnectionStateFailed {
			log.Println("Peer Connection failed")
			conn.Close()
		}
	})

	peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("Received track: ID=%s, Kind=%s, SSRC=%d",
			track.ID(), track.Kind(), track.SSRC())

		buf := make([]byte, 1500)
		bytesReceived := 0
		lastLog := 0

		for {
			n, _, readErr := track.Read(buf)
			if readErr != nil {
				log.Printf("Track read error: %v", readErr)
				return
			}
			bytesReceived += n
			if bytesReceived-lastLog > 1024*1024 {
				log.Printf("Track %s: received %.1f KB", track.ID(), float64(bytesReceived)/1024)
				lastLog = bytesReceived
			}
		}
	})

	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			_, msgData, err := conn.ReadMessage()
			if err != nil {
				log.Println("WebSocket read error:", err)
				return
			}

			var msg SignalingMessage
			if err := json.Unmarshal(msgData, &msg); err != nil {
				log.Println("Parse message error:", err)
				continue
			}

			switch msg.Type {
			case "offer":
				log.Println("Received SDP Offer")
				offer := webrtc.SessionDescription{
					Type: webrtc.SDPTypeOffer,
					SDP:  msg.SDP,
				}

				if err := peerConnection.SetRemoteDescription(offer); err != nil {
					log.Println("Set remote description error:", err)
					return
				}

				answer, err := peerConnection.CreateAnswer(nil)
				if err != nil {
					log.Println("Create answer error:", err)
					return
				}

				if err := peerConnection.SetLocalDescription(answer); err != nil {
					log.Println("Set local description error:", err)
					return
				}

				answerMsg := SignalingMessage{
					Type: "answer",
					SDP:  answer.SDP,
				}
				conn.WriteJSON(answerMsg)
				log.Println("Sent SDP Answer")

			case "ice":
				log.Println("Received ICE candidate")
				if msg.Candidate != nil {
					if err := peerConnection.AddICECandidate(*msg.Candidate); err != nil {
						log.Println("Add ICE candidate error:", err)
					}
				}
			}
		}
	}()

	<-done
	log.Println("Connection closed")
}

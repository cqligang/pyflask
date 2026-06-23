package webrtcutil

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/pion/webrtc/v3"
)

type SignalMsg struct {
	Type      string                   `json:"type"`
	SDP       string                   `json:"sdp,omitempty"`
	Candidate *webrtc.ICECandidateInit `json:"candidate,omitempty"`
}

func NewPeerConnection() (*webrtc.PeerConnection, error) {
	m := &webrtc.MediaEngine{}
	if err := m.RegisterDefaultCodecs(); err != nil {
		return nil, err
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(m))

	pc, err := api.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	})
	if err != nil {
		return nil, err
	}

	return pc, nil
}

func SetupConnectionCallbacks(pc *webrtc.PeerConnection, label string) {
	pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		fmt.Printf("[%s] Connection State: %s\n", label, s.String())
	})

	pc.OnICEConnectionStateChange(func(s webrtc.ICEConnectionState) {
		fmt.Printf("[%s] ICE State: %s\n", label, s.String())
	})

	pc.OnICEGatheringStateChange(func(s webrtc.ICEGathererState) {
		fmt.Printf("[%s] ICE Gathering: %s\n", label, s.String())
	})
}

func CollectICECandidates(pc *webrtc.PeerConnection, candidates *[]webrtc.ICECandidateInit) {
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		*candidates = append(*candidates, c.ToJSON())
		fmt.Printf("Got ICE candidate: %s\n", c.ToJSON().Candidate)
	})
}

func WaitForICEComplete(pc *webrtc.PeerConnection) {
	for {
		state := pc.ICEGatheringState()
		if state == webrtc.ICEGatheringStateComplete {
			return
		}
		if state == webrtc.ICEGatheringStateNew || state == webrtc.ICEGatheringStateGathering {
			time.Sleep(100 * time.Millisecond)
		} else {
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func ReadSDPFromFile(path string) (webrtc.SessionDescription, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return webrtc.SessionDescription{}, err
	}

	var msg SignalMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		return webrtc.SessionDescription{}, err
	}

	var sdpType webrtc.SDPType
	switch msg.Type {
	case "offer":
		sdpType = webrtc.SDPTypeOffer
	case "answer":
		sdpType = webrtc.SDPTypeAnswer
	default:
		return webrtc.SessionDescription{}, fmt.Errorf("unknown SDP type: %s", msg.Type)
	}

	return webrtc.SessionDescription{
		Type: sdpType,
		SDP:  msg.SDP,
	}, nil
}

func WriteSDPToFile(path string, sdpType string, sdp string) error {
	msg := SignalMsg{
		Type: sdpType,
		SDP:  sdp,
	}
	data, err := json.MarshalIndent(msg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func ReadICEFromFile(path string) ([]webrtc.ICECandidateInit, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var candidates []webrtc.ICECandidateInit
	if err := json.Unmarshal(data, &candidates); err != nil {
		return nil, err
	}
	return candidates, nil
}

func WriteICEToFile(path string, candidates []webrtc.ICECandidateInit) error {
	data, err := json.MarshalIndent(candidates, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func WaitForFile(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for file: %s", path)
}

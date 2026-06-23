package main

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"webrtc-go/pkg/webrtcutil"

	"github.com/pion/webrtc/v3"
)

func main() {
	fmt.Println("=== WebRTC Answer Side ===")

	fmt.Println("Waiting for offer.sdp...")
	err := webrtcutil.WaitForFile("offer.sdp", 60*time.Second)
	if err != nil {
		fmt.Printf("Timed out waiting for offer: %v\n", err)
		os.Exit(1)
	}

	offerSDP, err := webrtcutil.ReadSDPFromFile("offer.sdp")
	if err != nil {
		fmt.Printf("Failed to read offer SDP: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Offer SDP loaded")

	pc, err := webrtcutil.NewPeerConnection()
	if err != nil {
		fmt.Printf("Failed to create PeerConnection: %v\n", err)
		os.Exit(1)
	}
	defer pc.Close()

	webrtcutil.SetupConnectionCallbacks(pc, "ANSWER")

	stop := make(chan struct{})
	defer close(stop)

	videoTrack, err := webrtcutil.NewVideoTrack()
	if err != nil {
		fmt.Printf("Failed to create video track: %v\n", err)
		os.Exit(1)
	}

	audioTrack, err := webrtcutil.NewAudioTrack()
	if err != nil {
		fmt.Printf("Failed to create audio track: %v\n", err)
		os.Exit(1)
	}

	_, err = pc.AddTrack(videoTrack)
	if err != nil {
		fmt.Printf("Failed to add video track: %v\n", err)
		os.Exit(1)
	}

	_, err = pc.AddTrack(audioTrack)
	if err != nil {
		fmt.Printf("Failed to add audio track: %v\n", err)
		os.Exit(1)
	}

	var dc *webrtc.DataChannel
	pc.OnDataChannel(func(channel *webrtc.DataChannel) {
		fmt.Printf("DataChannel received: %s\n", channel.Label())
		dc = channel
		webrtcutil.SetupDataChannel(dc, "ANSWER", stop)
	})

	var iceCandidates []webrtc.ICECandidateInit
	webrtcutil.CollectICECandidates(pc, &iceCandidates)

	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		fmt.Printf("Received track: %s, kind: %s\n", track.ID(), track.Kind())
		if track.Kind() == webrtc.RTPCodecTypeVideo {
			go webrtcutil.ReceiveVideoTrack(track, "ANSWER", stop)
		} else {
			go webrtcutil.ReceiveAudioTrack(track, "ANSWER", stop)
		}
	})

	if err := pc.SetRemoteDescription(offerSDP); err != nil {
		fmt.Printf("Failed to set remote description: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Waiting for offer ICE candidates...")
	err = webrtcutil.WaitForFile("offer.ice", 30*time.Second)
	if err != nil {
		fmt.Printf("Warning: timed out waiting for offer ICE: %v\n", err)
	} else {
		offerICE, err := webrtcutil.ReadICEFromFile("offer.ice")
		if err != nil {
			fmt.Printf("Failed to read ICE candidates: %v\n", err)
		} else {
			for _, c := range offerICE {
				if err := pc.AddICECandidate(c); err != nil {
					fmt.Printf("Failed to add ICE candidate: %v\n", err)
				}
			}
			fmt.Printf("Added %d ICE candidates from offer\n", len(offerICE))
		}
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		fmt.Printf("Failed to create answer: %v\n", err)
		os.Exit(1)
	}

	if err := pc.SetLocalDescription(answer); err != nil {
		fmt.Printf("Failed to set local description: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Waiting for ICE gathering to complete...")
	webrtcutil.WaitForICEComplete(pc)
	fmt.Printf("Collected %d ICE candidates\n", len(iceCandidates))

	err = webrtcutil.WriteSDPToFile("answer.sdp", "answer", answer.SDP)
	if err != nil {
		fmt.Printf("Failed to write answer SDP: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Answer SDP written to answer.sdp")

	err = webrtcutil.WriteICEToFile("answer.ice", iceCandidates)
	if err != nil {
		fmt.Printf("Failed to write ICE candidates: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("ICE candidates written to answer.ice")

	go webrtcutil.GenerateVideoTestPattern(videoTrack, stop)
	go webrtcutil.GenerateAudioTone(audioTrack, stop)

	fmt.Println("\n=== WebRTC connection established! ===")
	fmt.Println("Type messages and press Enter to send via DataChannel")
	fmt.Println("Press Ctrl+C to exit\n")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			text := scanner.Text()
			if text == "" {
				continue
			}
			if dc != nil && dc.ReadyState() == webrtc.DataChannelStateOpen {
				dc.SendText(text)
				fmt.Printf("[YOU] %s\n", text)
			} else {
				fmt.Println("DataChannel not ready yet...")
			}
		}
	}()

	<-sigChan
	fmt.Println("\nShutting down...")

	os.Remove("offer.sdp")
	os.Remove("offer.ice")
	os.Remove("answer.sdp")
	os.Remove("answer.ice")
}

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
	fmt.Println("=== WebRTC Offer Side ===")

	pc, err := webrtcutil.NewPeerConnection()
	if err != nil {
		fmt.Printf("Failed to create PeerConnection: %v\n", err)
		os.Exit(1)
	}
	defer pc.Close()

	webrtcutil.SetupConnectionCallbacks(pc, "OFFER")

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

	dc, err := pc.CreateDataChannel("chat", nil)
	if err != nil {
		fmt.Printf("Failed to create DataChannel: %v\n", err)
		os.Exit(1)
	}
	webrtcutil.SetupDataChannel(dc, "OFFER", stop)

	var iceCandidates []webrtc.ICECandidateInit
	webrtcutil.CollectICECandidates(pc, &iceCandidates)

	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		fmt.Printf("Received track: %s, kind: %s\n", track.ID(), track.Kind())
		if track.Kind() == webrtc.RTPCodecTypeVideo {
			go webrtcutil.ReceiveVideoTrack(track, "OFFER", stop)
		} else {
			go webrtcutil.ReceiveAudioTrack(track, "OFFER", stop)
		}
	})

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		fmt.Printf("Failed to create offer: %v\n", err)
		os.Exit(1)
	}

	if err := pc.SetLocalDescription(offer); err != nil {
		fmt.Printf("Failed to set local description: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Waiting for ICE gathering to complete...")
	webrtcutil.WaitForICEComplete(pc)
	fmt.Printf("Collected %d ICE candidates\n", len(iceCandidates))

	err = webrtcutil.WriteSDPToFile("offer.sdp", "offer", offer.SDP)
	if err != nil {
		fmt.Printf("Failed to write offer SDP: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Offer SDP written to offer.sdp")

	err = webrtcutil.WriteICEToFile("offer.ice", iceCandidates)
	if err != nil {
		fmt.Printf("Failed to write ICE candidates: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("ICE candidates written to offer.ice")

	fmt.Println("\nNow run the answer side in another terminal.")
	fmt.Println("Waiting for answer.sdp...")

	err = webrtcutil.WaitForFile("answer.sdp", 60*time.Second)
	if err != nil {
		fmt.Printf("Timed out waiting for answer: %v\n", err)
		os.Exit(1)
	}

	answerSDP, err := webrtcutil.ReadSDPFromFile("answer.sdp")
	if err != nil {
		fmt.Printf("Failed to read answer SDP: %v\n", err)
		os.Exit(1)
	}

	if err := pc.SetRemoteDescription(answerSDP); err != nil {
		fmt.Printf("Failed to set remote description: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Answer SDP set successfully")

	fmt.Println("Waiting for answer.ice...")
	err = webrtcutil.WaitForFile("answer.ice", 30*time.Second)
	if err != nil {
		fmt.Printf("Warning: timed out waiting for ICE candidates: %v\n", err)
	} else {
		answerICE, err := webrtcutil.ReadICEFromFile("answer.ice")
		if err != nil {
			fmt.Printf("Failed to read ICE candidates: %v\n", err)
		} else {
			for _, c := range answerICE {
				if err := pc.AddICECandidate(c); err != nil {
					fmt.Printf("Failed to add ICE candidate: %v\n", err)
				}
			}
			fmt.Printf("Added %d ICE candidates from answer\n", len(answerICE))
		}
	}

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
			if dc.ReadyState() == webrtc.DataChannelStateOpen {
				dc.SendText(text)
				fmt.Printf("[YOU] %s\n", text)
			} else {
				fmt.Println("DataChannel not ready yet...")
			}
		}
	}()

	<-sigChan
	fmt.Println("\nShutting down...")
}

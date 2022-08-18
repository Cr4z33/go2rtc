package webrtc

import (
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/webrtc/v3"
)

const (
	MsgTypeOffer         = "webrtc/offer"
	MsgTypeOfferComplete = "webrtc/offer-complete"
	MsgTypeAnswer        = "webrtc/answer"
	MsgTypeCandidate     = "webrtc/candidate"
)

type Conn struct {
	streamer.Element

	UserAgent string

	Conn *webrtc.PeerConnection

	medias []*streamer.Media
	tracks []*streamer.Track

	receive int
	send    int
}

func (c *Conn) Init() {
	c.Conn.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil {
			c.Fire(&streamer.Message{
				Type: MsgTypeCandidate, Value: candidate.ToJSON().Candidate,
			})
		}
	})

	c.Conn.OnTrack(func(remote *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		for _, track := range c.tracks {
			if track.Direction != streamer.DirectionRecvonly {
				continue
			}
			if track.Codec.PayloadType != uint8(remote.PayloadType()) {
				continue
			}

			for {
				packet, _, err := remote.ReadRTP()
				if err != nil {
					return
				}
				if len(packet.Payload) == 0 {
					continue
				}
				c.receive += len(packet.Payload)
				_ = track.WriteRTP(packet)
			}
		}

		panic("something wrong")
	})

	c.Conn.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		c.Fire(state)

		// TODO: remove
		switch state {
		case webrtc.PeerConnectionStateConnected:
			c.Fire(streamer.StatePlaying)
		case webrtc.PeerConnectionStateDisconnected:
			c.Fire(streamer.StateNull)
		case webrtc.PeerConnectionStateFailed:
			_ = c.Conn.Close()
		}
	})
}

func (c *Conn) ExchangeSDP(offer string, complete bool) (answer string, err error) {
	sdOffer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer, SDP: offer,
	}
	if err = c.Conn.SetRemoteDescription(sdOffer); err != nil {
		return
	}

	//for _, tr := range c.Conn.GetTransceivers() {
	//	switch tr.Direction() {
	//	case webrtc.RTPTransceiverDirectionSendonly:
	//		// disable transceivers if we don't have track
	//		// make direction=inactive
	//		// don't really necessary, but anyway
	//		if tr.Sender() == nil {
	//			if err = tr.Stop(); err != nil {
	//				return
	//			}
	//		}
	//	case webrtc.RTPTransceiverDirectionRecvonly:
	//		// TODO: change codecs list
	//		caps := webrtc.RTPCodecCapability{
	//			MimeType:  webrtc.MimeTypePCMU,
	//			ClockRate: 8000,
	//		}
	//		codecs := []webrtc.RTPCodecParameters{
	//			{RTPCodecCapability: caps},
	//		}
	//		if err = tr.SetCodecPreferences(codecs); err != nil {
	//			return
	//		}
	//	}
	//}

	var sdAnswer webrtc.SessionDescription
	sdAnswer, err = c.Conn.CreateAnswer(nil)
	if err != nil {
		return
	}

	//var sd *sdp.SessionDescription
	//sd, err = sdAnswer.Unmarshal()
	//for _, media := range sd.MediaDescriptions {
	//	if media.MediaName.Media != "audio" {
	//		continue
	//	}
	//	for i, attr := range media.Attributes {
	//		if attr.Key == "sendonly" {
	//			attr.Key = "inactive"
	//			media.Attributes[i] = attr
	//			break
	//		}
	//	}
	//}
	//var b []byte
	//b, err = sd.Marshal()
	//sdAnswer.SDP = string(b)

	if err = c.Conn.SetLocalDescription(sdAnswer); err != nil {
		return
	}

	if complete {
		<-webrtc.GatheringCompletePromise(c.Conn)
		return c.Conn.LocalDescription().SDP, nil
	}

	return sdAnswer.SDP, nil
}

func (c *Conn) SetOffer(offer string) (err error) {
	sdOffer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer, SDP: offer,
	}
	if err = c.Conn.SetRemoteDescription(sdOffer); err != nil {
		return
	}
	rawSDP := []byte(c.Conn.RemoteDescription().SDP)
	c.medias, err = streamer.UnmarshalSDP(rawSDP)
	return
}

func (c *Conn) GetAnswer() (answer string, err error) {
	for _, tr := range c.Conn.GetTransceivers() {
		if tr.Direction() != webrtc.RTPTransceiverDirectionSendonly {
			continue
		}

		// disable transceivers if we don't have track
		// make direction=inactive
		// don't really necessary, but anyway
		if tr.Sender() == nil {
			if err = tr.Stop(); err != nil {
				return
			}
		}
	}

	var sdAnswer webrtc.SessionDescription
	sdAnswer, err = c.Conn.CreateAnswer(nil)
	if err != nil {
		return
	}

	if err = c.Conn.SetLocalDescription(sdAnswer); err != nil {
		return
	}

	return sdAnswer.SDP, nil
}

func (c *Conn) GetCompleteAnswer() (answer string, err error) {
	if _, err = c.GetAnswer(); err != nil {
		return
	}

	<-webrtc.GatheringCompletePromise(c.Conn)
	return c.Conn.LocalDescription().SDP, nil
}

func (c *Conn) remote() string {
	for _, trans := range c.Conn.GetTransceivers() {
		pair, _ := trans.Receiver().Transport().ICETransport().GetSelectedCandidatePair()
		return pair.Remote.String()
	}
	return ""
}

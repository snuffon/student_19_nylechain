package nylechain

/*
The service.go defines what to do for each API-call. This part of the service
runs on the node.
*/

import (
	"crypto/sha1"
	"errors"
	"hash"
	"time"

	"go.dedis.ch/cothority/v3/messaging"

	"github.com/dedis/student_19_nylechain/simpleblscosi"
	"go.dedis.ch/onet/v3"
	"go.dedis.ch/onet/v3/log"
	"go.dedis.ch/onet/v3/network"
)

// SimpleBLSCoSiID is used for tests
var SimpleBLSCoSiID onet.ServiceID

const protoName = "simpleBLSCoSi"

func init() {
	var err error
	if _, err := onet.GlobalProtocolRegister(protoName, simpleblscosi.NewDefaultProtocol); err != nil {
		log.ErrFatal(err)
	}
	SimpleBLSCoSiID, err = onet.RegisterNewService("SimpleBLSCoSi", newService)
	log.ErrFatal(err)
}

// Service structure
type Service struct {
	// We need to embed the ServiceProcessor, so that incoming messages
	// are correctly handled.
	*onet.ServiceProcessor

	hash1 hash.Hash

	propagateF messaging.PropagationFunc
}

// SimpleBLSCoSi starts a simpleblscosi-protocol and returns the final signature.
// The client chooses the message to be signed.
func (s *Service) SimpleBLSCoSi(cosi *CoSi) (*CoSiReply, error) {
	tree := cosi.Roster.GenerateNaryTreeWithRoot(2, s.ServerIdentity())
	if tree == nil {
		return nil, errors.New("couldn't create tree")
	}
	pi, err := s.CreateProtocol(protoName, tree)
	if err != nil {
		return nil, err
	}
	pi.(*simpleblscosi.SimpleBLSCoSi).Message = cosi.Message
	pi.Start()
	reply := &CoSiReply{
		Signature: <-pi.(*simpleblscosi.SimpleBLSCoSi).FinalSignature,
		Message:   cosi.Message,
	}
	s.startPropagation(s.propagateF, cosi.Roster, reply)
	return reply, nil
}

// propagateHandler receives a *CoSiReply containing both the initial message and the signature.
// It saves that CoSiReply in the service's bucket, keyed to a hash of the message.
func (s *Service) propagateHandler(msg network.Message) {
	message := msg.(*CoSiReply).Message
	s.hash1.Write(message)
	s.Save(s.hash1.Sum(nil), msg)
	return
}

func (s *Service) startPropagation(propagate messaging.PropagationFunc, ro *onet.Roster, msg network.Message) error {

	replies, err := propagate(ro, msg, 10*time.Second)
	if err != nil {
		return err
	}

	if replies != len(ro.List) {
		log.Lvl1(s.ServerIdentity(), "Only got", replies, "out of", len(ro.List))
	}

	return nil
}

// newService receives the context that holds information about the node it's
// running on. Saving and loading can be done using the context. The data will
// be stored in memory for tests and simulations, and on disk for real deployments.
func newService(c *onet.Context) (onet.Service, error) {
	s := &Service{
		ServiceProcessor: onet.NewServiceProcessor(c),
	}
	if err := s.RegisterHandler(s.SimpleBLSCoSi); err != nil {
		log.LLvl2(err)
		return nil, errors.New("Couldn't register message")
	}

	var err error
	s.propagateF, err = messaging.NewPropagationFunc(c, "Propagate", s.propagateHandler, -1)
	if err != nil {
		return nil, err
	}
	s.hash1 = sha1.New()
	s.GetAdditionalBucket([]byte("bucket"))
	return s, nil
}

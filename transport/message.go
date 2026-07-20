package transport

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"errors"
	"fmt"
	"unsafe"
)

const (
	MaxCompressedBits = 4096
	MsgMaxPlayers     = 4
)

const (
	Int64size = 8
	Int32size = 4
	Int16size = 2
	Int8size  = 1
)

func init() {
	gob.Register(&SyncRequestPacket{})
	gob.Register(&SyncReplyPacket{})
	gob.Register(&QualityReportPacket{})
	gob.Register(&QualityReplyPacket{})
	gob.Register(&InputPacket{})
	gob.Register(&InputAckPacket{})
	gob.Register(&KeepAlivePacket{})
}

type Message interface {
	Type() MessageType
	Header() Header
	SetHeader(magicNumber uint16, sequenceNumber uint16)
	PacketSize() int
	ToBytes() []byte
	FromBytes([]byte) error
}

type MessageType int

const (
	InvalidMsg MessageType = iota
	SyncRequestMsg
	SyncReplyMsg
	InputMsg
	QualityReportMsg
	QualityReplyMsg
	KeepAliveMsg
	InputAckMsg
)

type ConnectStatus struct {
	Disconnected bool
	LastFrame    int32
}

func (u *ConnectStatus) Size() int {
	sum := int(unsafe.Sizeof(u.Disconnected))
	sum += int(unsafe.Sizeof(u.LastFrame))
	return sum
}

func (u *ConnectStatus) ToBytes() []byte {
	buf := make([]byte, u.Size())
	if u.Disconnected {
		buf[0] = 1
	} else {
		buf[0] = 0
	}
	binary.BigEndian.PutUint32(buf[1:], uint32(u.LastFrame))
	return buf
}
func (u *ConnectStatus) FromBytes(buffer []byte) {
	if buffer[0] == 1 {
		u.Disconnected = true
	} else {
		u.Disconnected = false
	}
	u.LastFrame = int32(binary.BigEndian.Uint32(buffer[1:]))
}

type Header struct {
	Magic          uint16
	SequenceNumber uint16
	HeaderType     uint8
}

func (u Header) Size() int {
	sum := int(unsafe.Sizeof(u.Magic))
	sum += int(unsafe.Sizeof(u.SequenceNumber))
	sum += int(unsafe.Sizeof(u.HeaderType))
	return sum
}

func (u Header) ToBytes() []byte {
	buf := make([]byte, 5)
	binary.BigEndian.PutUint16(buf[:2], u.Magic)
	binary.BigEndian.PutUint16(buf[2:4], u.SequenceNumber)
	buf[4] = u.HeaderType
	return buf
}

func (u *Header) FromBytes(buffer []byte) {
	u.Magic = binary.BigEndian.Uint16(buffer[:2])
	u.SequenceNumber = binary.BigEndian.Uint16(buffer[2:4])
	u.HeaderType = buffer[4]
}

type SyncRequestPacket struct {
	MessageHeader    Header
	RandomRequest    uint32
	RemoteMagic      uint16
	RemoteEndpoint   uint8
	RemoteInputDelay uint8
}

func (s *SyncRequestPacket) Type() MessageType { return SyncRequestMsg }
func (s *SyncRequestPacket) Header() Header    { return s.MessageHeader }
func (s *SyncRequestPacket) SetHeader(magicNumber uint16, sequenceNumber uint16) {
	s.MessageHeader.Magic = magicNumber
	s.MessageHeader.SequenceNumber = sequenceNumber
}
func (s *SyncRequestPacket) PacketSize() int {
	sum := s.MessageHeader.Size()
	sum += int(unsafe.Sizeof(s.RandomRequest))
	sum += int(unsafe.Sizeof(s.RemoteMagic))
	sum += int(unsafe.Sizeof(s.RemoteEndpoint))
	sum += int(unsafe.Sizeof(s.RemoteInputDelay))
	return sum
}
func (s *SyncRequestPacket) String() string {
	return fmt.Sprintf("sync-request (%d).\n", s.RandomRequest)
}
func (s *SyncRequestPacket) ToBytes() []byte {
	buf := make([]byte, s.PacketSize())
	copy(buf, s.MessageHeader.ToBytes())
	binary.BigEndian.PutUint32(buf[5:9], s.RandomRequest)
	binary.BigEndian.PutUint16(buf[9:11], s.RemoteMagic)
	buf[11] = s.RemoteEndpoint
	buf[12] = s.RemoteInputDelay
	return buf
}

func (s *SyncRequestPacket) FromBytes(buffer []byte) error {
	if len(buffer) < s.PacketSize() {
		return errors.New("invalid packet")
	}
	s.MessageHeader.FromBytes(buffer)
	s.RandomRequest = binary.BigEndian.Uint32(buffer[5:9])
	s.RemoteMagic = binary.BigEndian.Uint16(buffer[9:11])
	s.RemoteEndpoint = buffer[11]
	s.RemoteInputDelay = buffer[12]
	return nil
}

type SyncReplyPacket struct {
	MessageHeader Header
	RandomReply   uint32
}

func (s *SyncReplyPacket) Type() MessageType { return SyncReplyMsg }
func (s *SyncReplyPacket) Header() Header    { return s.MessageHeader }
func (s *SyncReplyPacket) SetHeader(magicNumber uint16, sequenceNumber uint16) {
	s.MessageHeader.Magic = magicNumber
	s.MessageHeader.SequenceNumber = sequenceNumber
}
func (s *SyncReplyPacket) PacketSize() int {
	sum := s.MessageHeader.Size()
	sum += int(unsafe.Sizeof(s.RandomReply))
	return sum
}
func (s *SyncReplyPacket) String() string { return fmt.Sprintf("sync-reply (%d).\n", s.RandomReply) }

func (s *SyncReplyPacket) ToBytes() []byte {
	buf := make([]byte, s.PacketSize())
	copy(buf, s.MessageHeader.ToBytes())
	binary.BigEndian.PutUint32(buf[5:9], s.RandomReply)
	return buf
}

func (s *SyncReplyPacket) FromBytes(buffer []byte) error {
	if len(buffer) < s.PacketSize() {
		return errors.New("invalid packet")
	}
	s.MessageHeader.FromBytes(buffer)
	s.RandomReply = binary.BigEndian.Uint32(buffer[5:9])
	return nil
}

type QualityReportPacket struct {
	MessageHeader  Header
	FrameAdvantage int8
	Ping           uint64
}

func (q *QualityReportPacket) Type() MessageType { return QualityReportMsg }
func (q *QualityReportPacket) Header() Header    { return q.MessageHeader }
func (q *QualityReportPacket) SetHeader(magicNumber uint16, sequenceNumber uint16) {
	q.MessageHeader.Magic = magicNumber
	q.MessageHeader.SequenceNumber = sequenceNumber
}
func (q *QualityReportPacket) PacketSize() int {
	sum := q.MessageHeader.Size()
	sum += int(unsafe.Sizeof(q.FrameAdvantage))
	sum += int(unsafe.Sizeof(q.Ping))
	return sum
}

func (q *QualityReportPacket) String() string { return "quality report.\n" }

func (q *QualityReportPacket) ToBytes() []byte {
	buf := make([]byte, q.PacketSize())
	copy(buf, q.MessageHeader.ToBytes())
	buf[5] = uint8(q.FrameAdvantage)
	binary.BigEndian.PutUint64(buf[6:14], q.Ping)
	return buf
}

func (q *QualityReportPacket) FromBytes(buffer []byte) error {
	if len(buffer) < q.PacketSize() {
		return errors.New("invalid packet")
	}
	q.MessageHeader.FromBytes(buffer)
	q.FrameAdvantage = int8(buffer[5])
	q.Ping = binary.BigEndian.Uint64(buffer[6:14])
	return nil
}

type QualityReplyPacket struct {
	MessageHeader Header
	Pong          uint64
}

func (q *QualityReplyPacket) Type() MessageType { return QualityReplyMsg }
func (q *QualityReplyPacket) Header() Header    { return q.MessageHeader }
func (q *QualityReplyPacket) SetHeader(magicNumber uint16, sequenceNumber uint16) {
	q.MessageHeader.Magic = magicNumber
	q.MessageHeader.SequenceNumber = sequenceNumber
}
func (q *QualityReplyPacket) PacketSize() int {
	sum := q.MessageHeader.Size()
	sum += int(unsafe.Sizeof(q.Pong))
	return sum
}

func (q *QualityReplyPacket) String() string { return "quality reply.\n" }

func (q *QualityReplyPacket) ToBytes() []byte {
	buf := make([]byte, q.PacketSize())
	copy(buf, q.MessageHeader.ToBytes())
	binary.BigEndian.PutUint64(buf[5:13], q.Pong)
	return buf
}

func (q *QualityReplyPacket) FromBytes(buffer []byte) error {
	if len(buffer) < q.PacketSize() {
		return errors.New("invalid packet")
	}
	q.MessageHeader.FromBytes(buffer)
	q.Pong = binary.BigEndian.Uint64(buffer[5:13])
	return nil
}

type InputPacket struct {
	MessageHeader     Header
	PeerConnectStatus []ConnectStatus
	StartFrame        uint32

	DisconectRequested bool
	AckFrame           int32
	Checksum           uint32
	NumBits            uint16
	InputSize          uint8
	Bits               []byte
}

func (i *InputPacket) Type() MessageType { return InputMsg }
func (i *InputPacket) Header() Header    { return i.MessageHeader }
func (i *InputPacket) SetHeader(magicNumber uint16, sequenceNumber uint16) {
	i.MessageHeader.Magic = magicNumber
	i.MessageHeader.SequenceNumber = sequenceNumber
}

// may go back and make this calculate the Bits, Sizes and IsSpectator better
func (i *InputPacket) PacketSize() int {
	size := 0
	size += int(unsafe.Sizeof(i.MessageHeader))
	size += 1 // will store total
	for _, s := range i.PeerConnectStatus {
		size += s.Size()
	}
	size += int(unsafe.Sizeof(i.StartFrame))
	size += int(unsafe.Sizeof(i.DisconectRequested))
	size += int(unsafe.Sizeof(i.AckFrame))
	size += int(unsafe.Sizeof(i.Checksum))
	size += int(unsafe.Sizeof(i.NumBits))
	size += int(unsafe.Sizeof(i.InputSize))
	size += 1 // will store total
	size += len(i.Bits)
	//size += 1 // will store total
	return size
}
func (i *InputPacket) ToBytes() []byte {
	buf := make([]byte, i.PacketSize())
	copy(buf, i.MessageHeader.ToBytes())
	buf[5] = byte(len(i.PeerConnectStatus))
	offset := 6
	for _, p := range i.PeerConnectStatus {
		pcBuf := p.ToBytes()
		copy(buf[offset:offset+len(pcBuf)], pcBuf)
		offset += len(pcBuf)
	}
	binary.BigEndian.PutUint32(buf[offset:], i.StartFrame)
	offset += 4
	if i.DisconectRequested {
		buf[offset] = 1
	} else {
		buf[offset] = 0
	}
	offset++
	binary.BigEndian.PutUint32(buf[offset:], uint32(i.AckFrame))
	offset += Int32size
	binary.BigEndian.PutUint32(buf[offset:], i.Checksum)
	offset += Int32size
	binary.BigEndian.PutUint16(buf[offset:], i.NumBits)
	offset += 2
	buf[offset] = i.InputSize
	offset++
	buf[offset] = byte(len(i.Bits))
	offset++
	copy(buf[offset:offset+len(i.Bits)], i.Bits)
	offset += len(i.Bits)
	/*
		for _, input := range i.Bits {
			buf[offset] = byte(len(input))
			offset++
			copy(buf[offset:offset+len(input)], input)
			offset += len(input)
		}*/
	return buf
}

func (i *InputPacket) FromBytes(buffer []byte) error {
	if len(buffer) < i.PacketSize() {
		return errors.New("invalid packet")
	}

	i.MessageHeader.FromBytes(buffer)
	totalConnectionStatus := buffer[5]
	i.PeerConnectStatus = make([]ConnectStatus, totalConnectionStatus)
	pcsSize := i.PeerConnectStatus[0].Size()
	offset := 6
	for p := 0; p < int(totalConnectionStatus); p++ {
		i.PeerConnectStatus[p].FromBytes(buffer[offset : offset+pcsSize])
		offset += pcsSize
	}
	i.StartFrame = binary.BigEndian.Uint32(buffer[offset : offset+4])
	offset += 4
	if buffer[offset] == 1 {
		i.DisconectRequested = true
	} else if buffer[offset] == 0 {
		i.DisconectRequested = false
	}
	offset++
	i.AckFrame = int32(binary.BigEndian.Uint32(buffer[offset : offset+Int32size]))
	offset += Int32size
	i.Checksum = binary.BigEndian.Uint32(buffer[offset : offset+Int32size])
	offset += Int32size
	i.NumBits = binary.BigEndian.Uint16(buffer[offset : offset+2])
	offset += 2
	i.InputSize = buffer[offset]
	offset++
	totalBits := buffer[offset]
	offset++
	i.Bits = make([]byte, totalBits)
	copy(i.Bits, buffer[offset:offset+int(totalBits)])
	offset += int(totalBits)
	/*
		i.Bits = make([][]byte, totalBitsSlices)
		for b, _ := range i.Bits {
			curBitsSize := int(buffer[offset])
			offset++
			i.Bits[b] = buffer[offset : offset+curBitsSize]
			offset += curBitsSize
		}*/

	return nil
}

func (i InputPacket) String() string {
	return fmt.Sprintf("game-compressed-input %d (+ %d bits).\n",
		i.StartFrame, i.NumBits)
}

type InputAckPacket struct {
	MessageHeader Header
	AckFrame      int32
}

func (i *InputAckPacket) Type() MessageType { return InputAckMsg }
func (i *InputAckPacket) Header() Header    { return i.MessageHeader }
func (i *InputAckPacket) SetHeader(magicNumber uint16, sequenceNumber uint16) {
	i.MessageHeader.Magic = magicNumber
	i.MessageHeader.SequenceNumber = sequenceNumber
}
func (i *InputAckPacket) PacketSize() int {
	sum := i.MessageHeader.Size()
	sum += int(unsafe.Sizeof(i.AckFrame))
	return sum
}

func (i *InputAckPacket) ToBytes() []byte {
	buf := make([]byte, i.PacketSize())
	copy(buf, i.MessageHeader.ToBytes())
	binary.BigEndian.PutUint32(buf[5:], uint32(i.AckFrame))
	return buf
}

func (i *InputAckPacket) FromBytes(buffer []byte) error {
	if len(buffer) < i.PacketSize() {
		return errors.New("invalid packet")
	}
	i.MessageHeader.FromBytes(buffer)
	i.AckFrame = int32(binary.BigEndian.Uint32(buffer[5:]))
	return nil
}

func (i *InputAckPacket) String() string { return "input ack.\n" }

type KeepAlivePacket struct {
	MessageHeader Header
}

func (k *KeepAlivePacket) Type() MessageType { return KeepAliveMsg }
func (k *KeepAlivePacket) Header() Header    { return k.MessageHeader }
func (k *KeepAlivePacket) SetHeader(magicNumber uint16, sequenceNumber uint16) {
	k.MessageHeader.Magic = magicNumber
	k.MessageHeader.SequenceNumber = sequenceNumber
}
func (k *KeepAlivePacket) PacketSize() int {
	return k.MessageHeader.Size()
}
func (k *KeepAlivePacket) String() string { return "keep alive.\n" }

func (k *KeepAlivePacket) ToBytes() []byte {
	return k.MessageHeader.ToBytes()
}
func (k *KeepAlivePacket) FromBytes(buffer []byte) error {
	if len(buffer) < k.PacketSize() {
		return errors.New("invalid packet")
	}
	k.MessageHeader.FromBytes(buffer)
	return nil
}

func NewMessage(t MessageType) Message {
	header := Header{HeaderType: uint8(t)}
	var msg Message
	switch t {
	case SyncRequestMsg:
		msg = &SyncRequestPacket{
			MessageHeader: header}
	case SyncReplyMsg:
		msg = &SyncReplyPacket{
			MessageHeader: header}
	case QualityReportMsg:
		msg = &QualityReportPacket{
			MessageHeader: header}
	case QualityReplyMsg:
		msg = &QualityReplyPacket{
			MessageHeader: header}
	case InputAckMsg:
		msg = &InputAckPacket{
			MessageHeader: header}
	case InputMsg:
		msg = &InputPacket{
			MessageHeader: header}
	case KeepAliveMsg:
		fallthrough
	default:
		msg = &KeepAlivePacket{
			MessageHeader: header}
	}
	return msg
}

func EncodeMessage(packet Message) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	encErr := enc.Encode(&packet)
	if encErr != nil {
		return nil, encErr
	}
	return buf.Bytes(), nil
}

func DecodeMessage(buffer []byte) (Message, error) {
	buf := bytes.NewBuffer(buffer)
	dec := gob.NewDecoder(buf)
	var msg Message
	var err error

	if err = dec.Decode(&msg); err != nil {
		return nil, err
	}
	return msg, nil
}

func DecodeMessageBinary(buffer []byte) (Message, error) {
	msgType, err := GetPacketTypeFromBuffer(buffer)
	if err != nil {
		return nil, err
	}

	switch msgType {
	case SyncRequestMsg:
		var syncRequestPacket SyncRequestPacket
		err = syncRequestPacket.FromBytes(buffer)
		if err != nil {
			return nil, err
		}
		return &syncRequestPacket, nil
	case SyncReplyMsg:
		var syncReplyPacket SyncReplyPacket
		err = syncReplyPacket.FromBytes(buffer)
		if err != nil {
			return nil, err
		}
		return &syncReplyPacket, nil
	case QualityReportMsg:
		var qualityReportPacket QualityReportPacket
		err := qualityReportPacket.FromBytes(buffer)
		if err != nil {
			return nil, err
		}
		return &qualityReportPacket, nil
	case QualityReplyMsg:
		var qualityReplyPacket QualityReplyPacket
		err := qualityReplyPacket.FromBytes(buffer)
		if err != nil {
			return nil, err
		}
		return &qualityReplyPacket, nil
	case InputAckMsg:
		var inputAckPacket InputAckPacket
		err = inputAckPacket.FromBytes(buffer)
		if err != nil {
			return nil, err
		}
		return &inputAckPacket, nil
	case InputMsg:
		var inputPacket InputPacket
		err = inputPacket.FromBytes(buffer)
		if err != nil {
			return nil, err
		}
		return &inputPacket, nil
	case KeepAliveMsg:
		var keepAlivePacket KeepAlivePacket
		err = keepAlivePacket.FromBytes(buffer)
		if err != nil {
			return nil, err
		}
		return &keepAlivePacket, nil
	default:
		return nil, errors.New("message not recognized")
	}
}

func GetPacketTypeFromBuffer(buffer []byte) (MessageType, error) {
	if buffer == nil {
		return 0, errors.New("nil buffer")
	}
	if len(buffer) < 5 {
		return 0, errors.New("invalid header")
	}
	return MessageType(buffer[4]), nil
}

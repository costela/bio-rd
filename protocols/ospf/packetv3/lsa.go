package packetv3

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/bio-routing/bio-rd/net"
	"github.com/bio-routing/bio-rd/util/decode"
	"github.com/bio-routing/tflow2/convert"
	"github.com/pkg/errors"
)

type LSAType uint16

func (t LSAType) Serialize(buf *bytes.Buffer) {
	buf.Write(convert.Uint16Byte(uint16(t)))
}

func (t LSAType) FloodIfUnknown() bool {
	return t&(1<<15) != 0 // test for top bit
}

type FloodingScope uint8

const (
	FloodLinkLocal FloodingScope = iota
	FloodArea
	FloodAS
	FloodReserved
)

func (t LSAType) FloodingScope() FloodingScope {
	return FloodingScope((t & 0b0110000000000000) >> 13) // second and third bit as int
}

// OSPF LSA types
const (
	LSATypeUnknown         LSAType = 0
	LSATypeRouter                  = 0x2001
	LSATypeNetwork                 = 0x2002
	LSATypeInterAreaPrefix         = 0x2003
	LSATypeInterAreaRouter         = 0x2004
	LSATypeASExternal              = 0x4005
	LSATypeDeprecated              = 0x2006
	LSATypeNSSA                    = 0x2007
	LSATypeLink                    = 0x0008
	LSATypeIntraAreaPrefix         = 0x2009
)

type LSA struct {
	Age               uint16
	Type              LSAType
	ID                ID
	AdvertisingRouter ID
	SequenceNumber    uint32
	Checksum          uint16
	Length            uint16
	Body              Serializable
}

const LSAHeaderLength = 20

func (x *LSA) SerializeHeader(buf *bytes.Buffer) {
	buf.Write(convert.Uint16Byte(x.Age))
	x.Type.Serialize(buf)
	x.ID.Serialize(buf)
	x.AdvertisingRouter.Serialize(buf)
	buf.Write(convert.Uint32Byte(x.SequenceNumber))
	buf.Write(convert.Uint16Byte(x.Checksum))
	buf.Write(convert.Uint16Byte(x.Length))
}

func (x *LSA) Serialize(buf *bytes.Buffer) {
	x.SerializeHeader(buf)
	x.Body.Serialize(buf)
}

func DeserializeLSAHeader(buf *bytes.Buffer) (*LSA, int, error) {
	pdu := &LSA{}

	var readBytes int
	var err error
	var fields []interface{}

	fields = []interface{}{
		&pdu.Age,
		&pdu.Type,
		&pdu.ID,
		&pdu.AdvertisingRouter,
		&pdu.SequenceNumber,
		&pdu.Checksum,
		&pdu.Length,
	}

	err = decode.Decode(buf, fields)
	if err != nil {
		return nil, readBytes, fmt.Errorf("Unable to decode fields: %v", err)
	}
	readBytes += 20

	return pdu, readBytes, nil
}
func DeserializeLSA(buf *bytes.Buffer) (*LSA, int, error) {
	pdu, readBytes, err := DeserializeLSAHeader(buf)
	if err != nil {
		return nil, 0, err
	}

	n, err := pdu.ReadBody(buf)
	if err != nil {
		return nil, readBytes, errors.Wrap(err, "unable to decode LSA body")
	}
	readBytes += n

	return pdu, readBytes, nil
}

func (x *LSA) ReadBody(buf *bytes.Buffer) (int, error) {
	bodyLength := x.Length - LSAHeaderLength
	var body Serializable
	var readBytes int
	var err error

	switch x.Type {
	case LSATypeRouter:
		body, readBytes, err = DeserializeRouterLSA(buf, bodyLength)
	case LSATypeNetwork:
		body, readBytes, err = DeserializeNetworkLSA(buf, bodyLength)
	case LSATypeInterAreaPrefix:
		body, readBytes, err = DeserializeInterAreaPrefixLSA(buf)
	case LSATypeInterAreaRouter:
		body, readBytes, err = DeserializeInterAreaRouterLSA(buf)
	case LSATypeASExternal:
		body, readBytes, err = DeserializeASExternalLSA(buf)
	case LSATypeNSSA: // NSSA-LSA special case
		body, readBytes, err = DeserializeASExternalLSA(buf)
	case LSATypeLink:
		body, readBytes, err = DeserializeLinkLSA(buf)
	case LSATypeIntraAreaPrefix:
		body, readBytes, err = DeserializeIntraAreaPrefixLSA(buf)
	default:
		raw := make(UnknownLSA, bodyLength)
		readBytes, err = buf.Read(raw)
		body = raw
	}

	if err != nil {
		return readBytes, err
	}

	x.Body = body
	return readBytes, nil
}

type UnknownLSA []byte

func (u UnknownLSA) Serialize(buf *bytes.Buffer) {
	buf.Write(u)
}

// InterfaceMetric is the metric of a link
// This is supposed to be 24-bit long
type InterfaceMetric struct {
	High uint8
	Low  uint16
}

// Value returns the numeric value of this metric field
func (m InterfaceMetric) Value() uint32 {
	return uint32(m.High<<16) + uint32(m.Low)
}

func (x InterfaceMetric) Serialize(buf *bytes.Buffer) {
	buf.WriteByte(x.High)
	buf.Write(convert.Uint16Byte(x.Low))
}

type AreaLinkDescriptionType uint8

const (
	_ AreaLinkDescriptionType = iota
	ALDTypePTP
	ALDTypeTransit
	ALDTypeReserved
	ALDTypeVirtualLink
)

type AreaLinkDescription struct {
	Type                AreaLinkDescriptionType
	Metric              InterfaceMetric
	InterfaceID         ID
	NeighborInterfaceID ID
	NeighborRouterID    ID
}

func (x *AreaLinkDescription) Serialize(buf *bytes.Buffer) {
	buf.WriteByte(uint8(x.Type))
	x.Metric.Serialize(buf)
	x.InterfaceID.Serialize(buf)
	x.NeighborInterfaceID.Serialize(buf)
	x.NeighborRouterID.Serialize(buf)
}

func DeserializeAreaLinkDescription(buf *bytes.Buffer) (AreaLinkDescription, int, error) {
	pdu := AreaLinkDescription{}

	var readBytes int
	var err error
	var fields []interface{}

	fields = []interface{}{
		&pdu.Type,
		&pdu.Metric,
		&pdu.InterfaceID,
		&pdu.NeighborInterfaceID,
		&pdu.NeighborRouterID,
	}

	err = decode.Decode(buf, fields)
	if err != nil {
		return pdu, readBytes, fmt.Errorf("Unable to decode fields: %v", err)
	}
	readBytes += 16

	return pdu, readBytes, nil
}

type RouterLSAFlags uint8

const (
	RouterLSAFlagBorder RouterLSAFlags = 1 << iota
	RouterLSAFlagExternal
	RouterLSAFlagVirtualLink
	_
	RouterLSAFlagNSSATranslation
)

func RouterLSAFlagsFrom(flags ...RouterLSAFlags) RouterLSAFlags {
	var val RouterLSAFlags
	for _, flag := range flags {
		val = val | flag
	}
	return val
}

func (f RouterLSAFlags) HasFlag(flag uint8) bool {
	return uint8(f)&flag != 0
}

func (f RouterLSAFlags) SetFlag(flag uint8) uint8 {
	return uint8(f) | flag
}

type RouterLSA struct {
	Flags            RouterLSAFlags
	Options          RouterOptions
	LinkDescriptions []AreaLinkDescription
}

func (x *RouterLSA) Serialize(buf *bytes.Buffer) {
	buf.WriteByte(byte(x.Flags))
	x.Options.Serialize(buf)
	for i := range x.LinkDescriptions {
		x.LinkDescriptions[i].Serialize(buf)
	}
}

func DeserializeRouterLSA(buf *bytes.Buffer, bodyLength uint16) (*RouterLSA, int, error) {
	pdu := &RouterLSA{}

	var readBytes int
	var err error
	var fields []interface{}

	fields = []interface{}{
		&pdu.Flags,
		&pdu.Options,
	}

	err = decode.Decode(buf, fields)
	if err != nil {
		return nil, readBytes, fmt.Errorf("Unable to decode fields: %v", err)
	}
	readBytes += 4

	for i := readBytes; i < int(bodyLength); {
		tlv, n, err := DeserializeAreaLinkDescription(buf)
		if err != nil {
			return nil, readBytes, errors.Wrap(err, "unable to decode LinkDescription")
		}
		pdu.LinkDescriptions = append(pdu.LinkDescriptions, tlv)
		i += n
		readBytes += n
	}

	return pdu, readBytes, nil
}

type NetworkLSA struct {
	Options        RouterOptions
	AttachedRouter []ID
}

func (x *NetworkLSA) Serialize(buf *bytes.Buffer) {
	buf.WriteByte(0) // 1 byte reserved
	x.Options.Serialize(buf)
	for i := range x.AttachedRouter {
		x.AttachedRouter[i].Serialize(buf)
	}
}

func DeserializeNetworkLSA(buf *bytes.Buffer, bodyLength uint16) (*NetworkLSA, int, error) {
	pdu := &NetworkLSA{}

	var readBytes int
	var err error
	var fields []interface{}

	fields = []interface{}{
		new(uint8), // 1 byte reserved
		&pdu.Options,
	}

	err = decode.Decode(buf, fields)
	if err != nil {
		return nil, readBytes, fmt.Errorf("Unable to decode fields: %v", err)
	}
	readBytes += 4

	for i := readBytes; i < int(bodyLength); {
		tlv, n, err := DeserializeID(buf)
		if err != nil {
			return nil, 0, errors.Wrap(err, "Unable to decode AttachedRouterID")
		}
		pdu.AttachedRouter = append(pdu.AttachedRouter, tlv)
		i += n
		readBytes += n
	}

	return pdu, readBytes, nil
}

type InterAreaPrefixLSA struct {
	Metric InterfaceMetric
	Prefix LSAPrefix
}

func (x *InterAreaPrefixLSA) Serialize(buf *bytes.Buffer) {
	buf.WriteByte(0) // 1 byte reserved
	x.Metric.Serialize(buf)
	x.Prefix.Serialize(buf)
}

func DeserializeInterAreaPrefixLSA(buf *bytes.Buffer) (*InterAreaPrefixLSA, int, error) {
	pdu := &InterAreaPrefixLSA{}

	var readBytes int
	var err error
	var fields []interface{}

	fields = []interface{}{
		new(uint8), // 1 byte reserved
		&pdu.Metric,
	}

	err = decode.Decode(buf, fields)
	if err != nil {
		return nil, readBytes, fmt.Errorf("Unable to decode fields: %v", err)
	}
	readBytes += 4

	pfx, n, err := DeserializeLSAPrefix(buf)
	if err != nil {
		return nil, readBytes, errors.Wrap(err, "unable to decode prefix")
	}
	pdu.Prefix = pfx
	readBytes += n

	return pdu, readBytes, nil
}

type InterAreaRouterLSA struct {
	Options             RouterOptions
	Metric              InterfaceMetric
	DestinationRouterID ID
}

func (x *InterAreaRouterLSA) Serialize(buf *bytes.Buffer) {
	buf.WriteByte(0) // 1 byte reserved
	x.Options.Serialize(buf)
	buf.WriteByte(0) // 1 byte reserved
	x.Metric.Serialize(buf)
	x.DestinationRouterID.Serialize(buf)
}

func DeserializeInterAreaRouterLSA(buf *bytes.Buffer) (*InterAreaRouterLSA, int, error) {
	pdu := &InterAreaRouterLSA{}

	var readBytes int
	var err error
	var fields []interface{}

	fields = []interface{}{
		new(uint8), // 1 byte reserved
		&pdu.Options,
		new(uint8), // 1 byte reserved
		&pdu.Metric,
		&pdu.DestinationRouterID,
	}

	err = decode.Decode(buf, fields)
	if err != nil {
		return nil, readBytes, fmt.Errorf("Unable to decode fields: %v", err)
	}
	readBytes += 12

	return pdu, readBytes, nil
}

// Bitmasks for flags used in ASExternalLSA
const (
	ASExtLSAFlagT uint8 = 1 << iota
	ASExtLSAFlagF
	ASExtLSAFlagE
)

type ASExternalLSA struct {
	Flags  uint8
	Metric InterfaceMetric
	Prefix LSAPrefix

	ForwardingAddress     net.IP // optional
	ExternalRouteTag      uint32 // optional
	ReferencedLinkStateID ID     // optional
}

func (a *ASExternalLSA) FlagE() bool {
	return (a.Flags & ASExtLSAFlagE) != 0
}

func (a *ASExternalLSA) FlagF() bool {
	return (a.Flags & ASExtLSAFlagF) != 0
}

func (a *ASExternalLSA) FlagT() bool {
	return (a.Flags & ASExtLSAFlagT) != 0
}

func (x *ASExternalLSA) Serialize(buf *bytes.Buffer) {
	buf.WriteByte(x.Flags)
	x.Metric.Serialize(buf)
	x.Prefix.Serialize(buf)
	if x.FlagF() {
		serializeIPv6(x.ForwardingAddress, buf)
	}
	if x.FlagT() {
		buf.Write(convert.Uint32Byte(x.ExternalRouteTag))
	}
	if x.Prefix.Special != 0 {
		x.ReferencedLinkStateID.Serialize(buf)
	}
}

func DeserializeASExternalLSA(buf *bytes.Buffer) (*ASExternalLSA, int, error) {
	pdu := &ASExternalLSA{}

	var readBytes int
	var err error
	var fields []interface{}

	fields = []interface{}{
		&pdu.Flags,
		&pdu.Metric,
	}

	err = decode.Decode(buf, fields)
	if err != nil {
		return nil, readBytes, fmt.Errorf("Unable to decode fields: %v", err)
	}
	readBytes += 4

	pfx, n, err := DeserializeLSAPrefix(buf)
	if err != nil {
		return nil, readBytes, errors.Wrap(err, "unable to decode prefix")
	}
	pdu.Prefix = pfx
	readBytes += n

	if pdu.FlagF() {
		ip := deserializableIP{}
		err := binary.Read(buf, binary.BigEndian, &ip)
		if err != nil {
			return nil, readBytes, errors.Wrap(err, "unable to decode ForwardingAddress")
		}
		pdu.ForwardingAddress = ip.ToNetIP()
		readBytes += 16
	}
	if pdu.FlagT() {
		err := binary.Read(buf, binary.BigEndian, &pdu.ExternalRouteTag)
		if err != nil {
			return nil, readBytes, errors.Wrap(err, "unable to decode ExternalRouteTag")
		}
		readBytes += 4
	}
	if pdu.Prefix.Special != 0 {
		id, n, err := DeserializeID(buf)
		if err != nil {
			return nil, readBytes, errors.Wrap(err, "unable to decode ReferencedLinkStateID")
		}
		pdu.ReferencedLinkStateID = id
		readBytes += n
	}

	return pdu, readBytes, nil
}

type LinkLSA struct {
	RouterPriority            uint8
	Options                   RouterOptions
	LinkLocalInterfaceAddress net.IP
	PrefixNum                 uint32
	Prefixes                  []LSAPrefix
}

func (x *LinkLSA) Serialize(buf *bytes.Buffer) {
	buf.WriteByte(x.RouterPriority)
	x.Options.Serialize(buf)
	serializeIPv6(x.LinkLocalInterfaceAddress, buf)
	buf.Write(convert.Uint32Byte(x.PrefixNum))
	for i := range x.Prefixes {
		x.Prefixes[i].Serialize(buf)
	}
}

func DeserializeLinkLSA(buf *bytes.Buffer) (*LinkLSA, int, error) {
	pdu := &LinkLSA{}

	var readBytes int
	var err error
	var fields []interface{}

	llintfAddr := deserializableIP{}
	fields = []interface{}{
		&pdu.RouterPriority,
		&pdu.Options,
		&llintfAddr,
		&pdu.PrefixNum,
	}

	err = decode.Decode(buf, fields)
	if err != nil {
		return nil, readBytes, fmt.Errorf("Unable to decode fields: %v", err)
	}
	pdu.LinkLocalInterfaceAddress = llintfAddr.ToNetIP()
	readBytes += 24

	for i := 0; i < int(pdu.PrefixNum); i++ {
		tlv, n, err := DeserializeLSAPrefix(buf)
		if err != nil {
			return nil, 0, errors.Wrap(err, "Unable to decode")
		}
		pdu.Prefixes = append(pdu.Prefixes, tlv)
		readBytes += n
	}

	return pdu, readBytes, nil
}

type IntraAreaPrefixLSA struct {
	ReferencedLSType            LSAType
	ReferencedLinkStateID       ID
	ReferencedAdvertisingRouter ID
	Prefixes                    []LSAPrefix
}

func (x *IntraAreaPrefixLSA) Serialize(buf *bytes.Buffer) {
	buf.Write(convert.Uint16Byte(uint16(len(x.Prefixes))))
	x.ReferencedLSType.Serialize(buf)
	x.ReferencedLinkStateID.Serialize(buf)
	x.ReferencedAdvertisingRouter.Serialize(buf)
	for i := range x.Prefixes {
		x.Prefixes[i].Serialize(buf)
	}
}

func DeserializeIntraAreaPrefixLSA(buf *bytes.Buffer) (*IntraAreaPrefixLSA, int, error) {
	pdu := &IntraAreaPrefixLSA{}
	var prefixNum uint16

	var readBytes int
	var err error
	var fields []interface{}

	fields = []interface{}{
		&prefixNum,
		&pdu.ReferencedLSType,
		&pdu.ReferencedLinkStateID,
		&pdu.ReferencedAdvertisingRouter,
	}

	err = decode.Decode(buf, fields)
	if err != nil {
		return nil, readBytes, fmt.Errorf("Unable to decode fields: %v", err)
	}
	readBytes += 12

	for i := 0; i < int(prefixNum); i++ {
		tlv, n, err := DeserializeLSAPrefix(buf)
		if err != nil {
			return nil, 0, errors.Wrap(err, "Unable to decode")
		}
		pdu.Prefixes = append(pdu.Prefixes, tlv)
		readBytes += n
	}

	return pdu, readBytes, nil
}
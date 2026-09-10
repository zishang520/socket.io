package parser

import "github.com/zishang520/socket.io/v3/pkg/types"

// DecoderOptions holds configuration options for the Decoder.
type (
	DecoderOptionsInterface interface {
		SetMaxAttachments(uint64)
		GetRawMaxAttachments() types.Optional[uint64]
		MaxAttachments() uint64

		SetMaxNamespaceLength(int)
		GetRawMaxNamespaceLength() types.Optional[int]
		MaxNamespaceLength() int

		SetMaxPacketIDLength(int)
		GetRawMaxPacketIDLength() types.Optional[int]
		MaxPacketIDLength() int
	}

	DecoderOptions struct {
		// MaxAttachments is the maximum number of binary attachments allowed per packet.
		// Defaults to DefaultMaxAttachments (10) if not set or set to 0.
		maxAttachments types.Optional[uint64]

		// MaxNamespaceLength is the maximum allowed length of a namespace name.
		// Defaults to DefaultMaxNamespaceLength (512) if not set or non-positive.
		maxNamespaceLength types.Optional[int]

		// MaxPacketIDLength is the maximum allowed length of a packet ID string.
		// Defaults to DefaultMaxPacketIDLength (20) if not set or non-positive.
		maxPacketIDLength types.Optional[int]
	}
)

func DefaultDecoderOptions() *DecoderOptions {
	return &DecoderOptions{}
}

func (d *DecoderOptions) Assign(data DecoderOptionsInterface) *DecoderOptions {
	if data == nil {
		return d
	}

	if data.GetRawMaxAttachments() != nil {
		d.SetMaxAttachments(data.MaxAttachments())
	}

	if data.GetRawMaxNamespaceLength() != nil {
		d.SetMaxNamespaceLength(data.MaxNamespaceLength())
	}

	if data.GetRawMaxPacketIDLength() != nil {
		d.SetMaxPacketIDLength(data.MaxPacketIDLength())
	}

	return d
}

// MaxAttachments is the maximum number of binary attachments allowed per packet.
func (d *DecoderOptions) SetMaxAttachments(maxAttachments uint64) {
	d.maxAttachments = types.NewSome(maxAttachments)
}
func (d *DecoderOptions) GetRawMaxAttachments() types.Optional[uint64] {
	return d.maxAttachments
}
func (d *DecoderOptions) MaxAttachments() uint64 {
	if d.maxAttachments == nil {
		return 0
	}
	return d.maxAttachments.Get()
}

// MaxNamespaceLength is the maximum allowed length of a namespace name.
func (d *DecoderOptions) SetMaxNamespaceLength(maxNamespaceLength int) {
	d.maxNamespaceLength = types.NewSome(maxNamespaceLength)
}
func (d *DecoderOptions) GetRawMaxNamespaceLength() types.Optional[int] {
	return d.maxNamespaceLength
}
func (d *DecoderOptions) MaxNamespaceLength() int {
	if d.maxNamespaceLength == nil {
		return 0
	}
	return d.maxNamespaceLength.Get()
}

// MaxPacketIDLength is the maximum allowed length of a packet ID string.
func (d *DecoderOptions) SetMaxPacketIDLength(maxPacketIDLength int) {
	d.maxPacketIDLength = types.NewSome(maxPacketIDLength)
}
func (d *DecoderOptions) GetRawMaxPacketIDLength() types.Optional[int] {
	return d.maxPacketIDLength
}
func (d *DecoderOptions) MaxPacketIDLength() int {
	if d.maxPacketIDLength == nil {
		return 0
	}
	return d.maxPacketIDLength.Get()
}

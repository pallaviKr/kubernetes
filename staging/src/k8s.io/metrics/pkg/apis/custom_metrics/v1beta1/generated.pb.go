/*
Copyright The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: k8s.io/kubernetes/vendor/k8s.io/metrics/pkg/apis/custom_metrics/v1beta1/generated.proto

package v1beta1

import (
	fmt "fmt"

	io "io"

	proto "github.com/gogo/protobuf/proto"
	v11 "k8s.io/apimachinery/pkg/apis/meta/v1"

	math "math"
	math_bits "math/bits"
	reflect "reflect"
	strings "strings"
)

// Reference imports to suppress errors if they are not otherwise used.
var (
	_ = proto.Marshal
	_ = fmt.Errorf
	_ = math.Inf
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.GoGoProtoPackageIsVersion3 // please upgrade the proto package

func (m *MetricListOptions) Reset()      { *m = MetricListOptions{} }
func (*MetricListOptions) ProtoMessage() {}
func (*MetricListOptions) Descriptor() ([]byte, []int) {
	return fileDescriptor_eb308345182a1e5e, []int{0}
}

func (m *MetricListOptions) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}

func (m *MetricListOptions) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	b = b[:cap(b)]
	n, err := m.MarshalToSizedBuffer(b)
	if err != nil {
		return nil, err
	}
	return b[:n], nil
}

func (m *MetricListOptions) XXX_Merge(src proto.Message) {
	xxx_messageInfo_MetricListOptions.Merge(m, src)
}

func (m *MetricListOptions) XXX_Size() int {
	return m.Size()
}

func (m *MetricListOptions) XXX_DiscardUnknown() {
	xxx_messageInfo_MetricListOptions.DiscardUnknown(m)
}

var xxx_messageInfo_MetricListOptions proto.InternalMessageInfo

func (m *MetricValue) Reset()      { *m = MetricValue{} }
func (*MetricValue) ProtoMessage() {}
func (*MetricValue) Descriptor() ([]byte, []int) {
	return fileDescriptor_eb308345182a1e5e, []int{1}
}

func (m *MetricValue) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}

func (m *MetricValue) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	b = b[:cap(b)]
	n, err := m.MarshalToSizedBuffer(b)
	if err != nil {
		return nil, err
	}
	return b[:n], nil
}

func (m *MetricValue) XXX_Merge(src proto.Message) {
	xxx_messageInfo_MetricValue.Merge(m, src)
}

func (m *MetricValue) XXX_Size() int {
	return m.Size()
}

func (m *MetricValue) XXX_DiscardUnknown() {
	xxx_messageInfo_MetricValue.DiscardUnknown(m)
}

var xxx_messageInfo_MetricValue proto.InternalMessageInfo

func (m *MetricValueList) Reset()      { *m = MetricValueList{} }
func (*MetricValueList) ProtoMessage() {}
func (*MetricValueList) Descriptor() ([]byte, []int) {
	return fileDescriptor_eb308345182a1e5e, []int{2}
}

func (m *MetricValueList) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}

func (m *MetricValueList) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	b = b[:cap(b)]
	n, err := m.MarshalToSizedBuffer(b)
	if err != nil {
		return nil, err
	}
	return b[:n], nil
}

func (m *MetricValueList) XXX_Merge(src proto.Message) {
	xxx_messageInfo_MetricValueList.Merge(m, src)
}

func (m *MetricValueList) XXX_Size() int {
	return m.Size()
}

func (m *MetricValueList) XXX_DiscardUnknown() {
	xxx_messageInfo_MetricValueList.DiscardUnknown(m)
}

var xxx_messageInfo_MetricValueList proto.InternalMessageInfo

func init() {
	proto.RegisterType((*MetricListOptions)(nil), "k8s.io.metrics.pkg.apis.custom_metrics.v1beta1.MetricListOptions")
	proto.RegisterType((*MetricValue)(nil), "k8s.io.metrics.pkg.apis.custom_metrics.v1beta1.MetricValue")
	proto.RegisterType((*MetricValueList)(nil), "k8s.io.metrics.pkg.apis.custom_metrics.v1beta1.MetricValueList")
}

func init() {
	proto.RegisterFile("k8s.io/kubernetes/vendor/k8s.io/metrics/pkg/apis/custom_metrics/v1beta1/generated.proto", fileDescriptor_eb308345182a1e5e)
}

var fileDescriptor_eb308345182a1e5e = []byte{
	// 616 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x94, 0x94, 0x4f, 0x4f, 0x14, 0x3f,
	0x1c, 0xc6, 0x77, 0xd8, 0xdf, 0xf2, 0x5b, 0x8a, 0x04, 0x19, 0x62, 0xdc, 0x60, 0x32, 0x90, 0xf5,
	0x82, 0x26, 0xb4, 0x01, 0x8d, 0x31, 0xe1, 0x36, 0xf1, 0x62, 0xc2, 0x4a, 0x1c, 0x88, 0x24, 0xfe,
	0x89, 0x76, 0x3a, 0x5f, 0x96, 0xba, 0x3b, 0xd3, 0x49, 0xdb, 0x59, 0xc2, 0xcd, 0x97, 0xe0, 0x3b,
	0xf0, 0xed, 0x70, 0xc4, 0x1b, 0x27, 0x22, 0x63, 0x7c, 0x1f, 0x66, 0x3a, 0x9d, 0xfd, 0xc3, 0xa2,
	0xc2, 0x6d, 0xa7, 0x7d, 0x9e, 0x4f, 0x9f, 0x7e, 0x9f, 0x66, 0xd1, 0x41, 0xef, 0xb9, 0xc2, 0x5c,
	0x90, 0x5e, 0x16, 0x82, 0x4c, 0x40, 0x83, 0x22, 0x03, 0x48, 0x22, 0x21, 0x89, 0xdd, 0x88, 0x41,
	0x4b, 0xce, 0x14, 0x49, 0x7b, 0x5d, 0x42, 0x53, 0xae, 0x08, 0xcb, 0x94, 0x16, 0xf1, 0xc7, 0x6a,
	0x7d, 0xb0, 0x19, 0x82, 0xa6, 0x9b, 0xa4, 0x0b, 0x09, 0x48, 0xaa, 0x21, 0xc2, 0xa9, 0x14, 0x5a,
	0xb8, 0xb8, 0xf4, 0x63, 0xab, 0xc3, 0x69, 0xaf, 0x8b, 0x0b, 0x3f, 0x9e, 0xf4, 0x63, 0xeb, 0x5f,
	0xd9, 0xe8, 0x72, 0x7d, 0x94, 0x85, 0x98, 0x89, 0x98, 0x74, 0x45, 0x57, 0x10, 0x83, 0x09, 0xb3,
	0x43, 0xf3, 0x65, 0x3e, 0xcc, 0xaf, 0x12, 0xbf, 0xd2, 0xb6, 0xf1, 0x68, 0xca, 0x09, 0x13, 0x12,
	0xc8, 0x60, 0x2a, 0xc2, 0xca, 0xd3, 0x91, 0x26, 0xa6, 0xec, 0x88, 0x27, 0x20, 0x4f, 0xaa, 0x7b,
	0x10, 0x09, 0x4a, 0x64, 0x92, 0xc1, 0xad, 0x5c, 0xaa, 0x18, 0x07, 0xbd, 0xee, 0x2c, 0xf2, 0x27,
	0x97, 0xcc, 0x12, 0xcd, 0xe3, 0xe9, 0x63, 0x9e, 0xfd, 0xcb, 0xa0, 0xd8, 0x11, 0xc4, 0xf4, 0xaa,
	0xaf, 0xfd, 0xcd, 0x41, 0x4b, 0x1d, 0x33, 0xbb, 0x1d, 0xae, 0xf4, 0x6e, 0xaa, 0xb9, 0x48, 0x94,
	0xbb, 0x8d, 0x16, 0xfa, 0x34, 0x84, 0xfe, 0x1e, 0xf4, 0x81, 0x69, 0x21, 0x5b, 0xce, 0x9a, 0xb3,
	0x3e, 0xe7, 0xdf, 0x3b, 0xbd, 0x58, 0xad, 0xe5, 0x17, 0xab, 0x0b, 0x3b, 0xe3, 0x9b, 0xc1, 0xa4,
	0xd6, 0xed, 0xa0, 0xe5, 0xb2, 0x8d, 0x09, 0x55, 0x6b, 0xc6, 0x20, 0x1e, 0x58, 0xc4, 0x72, 0x67,
	0x5a, 0x12, 0x5c, 0xe7, 0x6b, 0xff, 0xaa, 0xa3, 0xf9, 0x52, 0xfc, 0x86, 0xf6, 0x33, 0x70, 0x0f,
	0xd1, 0x62, 0x04, 0x8a, 0x49, 0x1e, 0x42, 0xb4, 0x1b, 0x7e, 0x06, 0xa6, 0x4d, 0xba, 0xf9, 0xad,
	0x87, 0xd5, 0x1b, 0xa1, 0x29, 0xc7, 0x45, 0x89, 0x78, 0xb0, 0x89, 0x4b, 0x45, 0x00, 0x87, 0x20,
	0x21, 0x61, 0xe0, 0xdf, 0xb7, 0xe7, 0x2f, 0xbe, 0x98, 0x64, 0x04, 0x57, 0xa1, 0xee, 0x16, 0x42,
	0x65, 0x9c, 0x57, 0x34, 0x06, 0x9b, 0xde, 0xb5, 0x6e, 0xd4, 0x19, 0xee, 0x04, 0x63, 0x2a, 0xf7,
	0x1d, 0x9a, 0x2b, 0x86, 0xad, 0x34, 0x8d, 0xd3, 0x56, 0xdd, 0xa4, 0x7a, 0x3c, 0x96, 0x6a, 0xd8,
	0xcc, 0xe8, 0xf9, 0x16, 0x0f, 0xa0, 0xc8, 0xb9, 0xcf, 0x63, 0xf0, 0x97, 0x2c, 0x7e, 0x6e, 0xbf,
	0x82, 0x04, 0x23, 0x9e, 0xfb, 0x08, 0xcd, 0x1e, 0xf3, 0x24, 0x12, 0xc7, 0xad, 0xff, 0xd6, 0x9c,
	0xf5, 0xba, 0xbf, 0x54, 0x34, 0x71, 0x60, 0x56, 0xf6, 0x80, 0x89, 0x24, 0x52, 0x81, 0x15, 0xb8,
	0x7b, 0xa8, 0x31, 0x28, 0x86, 0xd5, 0x6a, 0x98, 0x0c, 0xf8, 0x6f, 0x19, 0x70, 0xf5, 0x74, 0xf1,
	0xeb, 0x8c, 0x26, 0x9a, 0xeb, 0x13, 0x7f, 0xc1, 0xe6, 0x68, 0x98, 0x89, 0x07, 0x25, 0xcb, 0xfd,
	0x80, 0x9a, 0xaa, 0x2a, 0x73, 0xd6, 0x70, 0x9f, 0xdc, 0xec, 0x6e, 0x13, 0x7d, 0xfa, 0x77, 0xf2,
	0x8b, 0xd5, 0xe6, 0xb0, 0xf2, 0x21, 0xb2, 0xfd, 0xdd, 0x41, 0x8b, 0x63, 0x3d, 0x17, 0xcf, 0xd1,
	0x7d, 0x8f, 0x9a, 0x05, 0x24, 0xa2, 0x9a, 0xda, 0x92, 0xf1, 0x0d, 0x8f, 0xe4, 0x4a, 0x77, 0x40,
	0x53, 0xff, 0xae, 0xbd, 0x4a, 0xb3, 0x5a, 0x09, 0x86, 0x44, 0xf7, 0x13, 0x6a, 0x70, 0x0d, 0xb1,
	0x6a, 0xcd, 0xac, 0xd5, 0xd7, 0xe7, 0xb7, 0xb6, 0x6f, 0xf9, 0x1f, 0x83, 0xc7, 0xd2, 0x8e, 0x46,
	0xf6, 0xb2, 0x20, 0x06, 0x25, 0xd8, 0xdf, 0x38, 0xbd, 0xf4, 0x6a, 0x67, 0x97, 0x5e, 0xed, 0xfc,
	0xd2, 0xab, 0x7d, 0xc9, 0x3d, 0xe7, 0x34, 0xf7, 0x9c, 0xb3, 0xdc, 0x73, 0xce, 0x73, 0xcf, 0xf9,
	0x91, 0x7b, 0xce, 0xd7, 0x9f, 0x5e, 0xed, 0xed, 0xff, 0x16, 0xf8, 0x3b, 0x00, 0x00, 0xff, 0xff,
	0xf5, 0x23, 0xb5, 0xdc, 0x3e, 0x05, 0x00, 0x00,
}

func (m *MetricListOptions) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *MetricListOptions) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *MetricListOptions) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	i -= len(m.MetricLabelSelector)
	copy(dAtA[i:], m.MetricLabelSelector)
	i = encodeVarintGenerated(dAtA, i, uint64(len(m.MetricLabelSelector)))
	i--
	dAtA[i] = 0x12
	i -= len(m.LabelSelector)
	copy(dAtA[i:], m.LabelSelector)
	i = encodeVarintGenerated(dAtA, i, uint64(len(m.LabelSelector)))
	i--
	dAtA[i] = 0xa
	return len(dAtA) - i, nil
}

func (m *MetricValue) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *MetricValue) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *MetricValue) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if m.Selector != nil {
		{
			size, err := m.Selector.MarshalToSizedBuffer(dAtA[:i])
			if err != nil {
				return 0, err
			}
			i -= size
			i = encodeVarintGenerated(dAtA, i, uint64(size))
		}
		i--
		dAtA[i] = 0x32
	}
	{
		size, err := m.Value.MarshalToSizedBuffer(dAtA[:i])
		if err != nil {
			return 0, err
		}
		i -= size
		i = encodeVarintGenerated(dAtA, i, uint64(size))
	}
	i--
	dAtA[i] = 0x2a
	if m.WindowSeconds != nil {
		i = encodeVarintGenerated(dAtA, i, uint64(*m.WindowSeconds))
		i--
		dAtA[i] = 0x20
	}
	{
		size, err := m.Timestamp.MarshalToSizedBuffer(dAtA[:i])
		if err != nil {
			return 0, err
		}
		i -= size
		i = encodeVarintGenerated(dAtA, i, uint64(size))
	}
	i--
	dAtA[i] = 0x1a
	i -= len(m.MetricName)
	copy(dAtA[i:], m.MetricName)
	i = encodeVarintGenerated(dAtA, i, uint64(len(m.MetricName)))
	i--
	dAtA[i] = 0x12
	{
		size, err := m.DescribedObject.MarshalToSizedBuffer(dAtA[:i])
		if err != nil {
			return 0, err
		}
		i -= size
		i = encodeVarintGenerated(dAtA, i, uint64(size))
	}
	i--
	dAtA[i] = 0xa
	return len(dAtA) - i, nil
}

func (m *MetricValueList) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *MetricValueList) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *MetricValueList) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if len(m.Items) > 0 {
		for iNdEx := len(m.Items) - 1; iNdEx >= 0; iNdEx-- {
			{
				size, err := m.Items[iNdEx].MarshalToSizedBuffer(dAtA[:i])
				if err != nil {
					return 0, err
				}
				i -= size
				i = encodeVarintGenerated(dAtA, i, uint64(size))
			}
			i--
			dAtA[i] = 0x12
		}
	}
	{
		size, err := m.ListMeta.MarshalToSizedBuffer(dAtA[:i])
		if err != nil {
			return 0, err
		}
		i -= size
		i = encodeVarintGenerated(dAtA, i, uint64(size))
	}
	i--
	dAtA[i] = 0xa
	return len(dAtA) - i, nil
}

func encodeVarintGenerated(dAtA []byte, offset int, v uint64) int {
	offset -= sovGenerated(v)
	base := offset
	for v >= 1<<7 {
		dAtA[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	dAtA[offset] = uint8(v)
	return base
}

func (m *MetricListOptions) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	l = len(m.LabelSelector)
	n += 1 + l + sovGenerated(uint64(l))
	l = len(m.MetricLabelSelector)
	n += 1 + l + sovGenerated(uint64(l))
	return n
}

func (m *MetricValue) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	l = m.DescribedObject.Size()
	n += 1 + l + sovGenerated(uint64(l))
	l = len(m.MetricName)
	n += 1 + l + sovGenerated(uint64(l))
	l = m.Timestamp.Size()
	n += 1 + l + sovGenerated(uint64(l))
	if m.WindowSeconds != nil {
		n += 1 + sovGenerated(uint64(*m.WindowSeconds))
	}
	l = m.Value.Size()
	n += 1 + l + sovGenerated(uint64(l))
	if m.Selector != nil {
		l = m.Selector.Size()
		n += 1 + l + sovGenerated(uint64(l))
	}
	return n
}

func (m *MetricValueList) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	l = m.ListMeta.Size()
	n += 1 + l + sovGenerated(uint64(l))
	if len(m.Items) > 0 {
		for _, e := range m.Items {
			l = e.Size()
			n += 1 + l + sovGenerated(uint64(l))
		}
	}
	return n
}

func sovGenerated(x uint64) (n int) {
	return (math_bits.Len64(x|1) + 6) / 7
}

func sozGenerated(x uint64) (n int) {
	return sovGenerated(uint64((x << 1) ^ uint64((int64(x) >> 63))))
}

func (this *MetricListOptions) String() string {
	if this == nil {
		return "nil"
	}
	s := strings.Join([]string{
		`&MetricListOptions{`,
		`LabelSelector:` + fmt.Sprintf("%v", this.LabelSelector) + `,`,
		`MetricLabelSelector:` + fmt.Sprintf("%v", this.MetricLabelSelector) + `,`,
		`}`,
	}, "")
	return s
}

func (this *MetricValue) String() string {
	if this == nil {
		return "nil"
	}
	s := strings.Join([]string{
		`&MetricValue{`,
		`DescribedObject:` + strings.Replace(strings.Replace(fmt.Sprintf("%v", this.DescribedObject), "ObjectReference", "v1.ObjectReference", 1), `&`, ``, 1) + `,`,
		`MetricName:` + fmt.Sprintf("%v", this.MetricName) + `,`,
		`Timestamp:` + strings.Replace(strings.Replace(fmt.Sprintf("%v", this.Timestamp), "Time", "v11.Time", 1), `&`, ``, 1) + `,`,
		`WindowSeconds:` + valueToStringGenerated(this.WindowSeconds) + `,`,
		`Value:` + strings.Replace(strings.Replace(fmt.Sprintf("%v", this.Value), "Quantity", "resource.Quantity", 1), `&`, ``, 1) + `,`,
		`Selector:` + strings.Replace(fmt.Sprintf("%v", this.Selector), "LabelSelector", "v11.LabelSelector", 1) + `,`,
		`}`,
	}, "")
	return s
}

func (this *MetricValueList) String() string {
	if this == nil {
		return "nil"
	}
	repeatedStringForItems := "[]MetricValue{"
	for _, f := range this.Items {
		repeatedStringForItems += strings.Replace(strings.Replace(f.String(), "MetricValue", "MetricValue", 1), `&`, ``, 1) + ","
	}
	repeatedStringForItems += "}"
	s := strings.Join([]string{
		`&MetricValueList{`,
		`ListMeta:` + strings.Replace(strings.Replace(fmt.Sprintf("%v", this.ListMeta), "ListMeta", "v11.ListMeta", 1), `&`, ``, 1) + `,`,
		`Items:` + repeatedStringForItems + `,`,
		`}`,
	}, "")
	return s
}

func valueToStringGenerated(v interface{}) string {
	rv := reflect.ValueOf(v)
	if rv.IsNil() {
		return "nil"
	}
	pv := reflect.Indirect(rv).Interface()
	return fmt.Sprintf("*%v", pv)
}

func (m *MetricListOptions) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowGenerated
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: MetricListOptions: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: MetricListOptions: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field LabelSelector", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowGenerated
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthGenerated
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return ErrInvalidLengthGenerated
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.LabelSelector = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field MetricLabelSelector", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowGenerated
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthGenerated
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return ErrInvalidLengthGenerated
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.MetricLabelSelector = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipGenerated(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if (skippy < 0) || (iNdEx+skippy) < 0 {
				return ErrInvalidLengthGenerated
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}

func (m *MetricValue) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowGenerated
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: MetricValue: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: MetricValue: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field DescribedObject", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowGenerated
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthGenerated
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthGenerated
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if err := m.DescribedObject.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field MetricName", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowGenerated
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthGenerated
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return ErrInvalidLengthGenerated
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.MetricName = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 3:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Timestamp", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowGenerated
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthGenerated
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthGenerated
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if err := m.Timestamp.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 4:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field WindowSeconds", wireType)
			}
			var v int64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowGenerated
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				v |= int64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			m.WindowSeconds = &v
		case 5:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Value", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowGenerated
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthGenerated
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthGenerated
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if err := m.Value.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 6:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Selector", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowGenerated
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthGenerated
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthGenerated
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if m.Selector == nil {
				m.Selector = &v11.LabelSelector{}
			}
			if err := m.Selector.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipGenerated(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if (skippy < 0) || (iNdEx+skippy) < 0 {
				return ErrInvalidLengthGenerated
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}

func (m *MetricValueList) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowGenerated
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: MetricValueList: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: MetricValueList: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field ListMeta", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowGenerated
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthGenerated
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthGenerated
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if err := m.ListMeta.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Items", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowGenerated
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthGenerated
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthGenerated
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Items = append(m.Items, MetricValue{})
			if err := m.Items[len(m.Items)-1].Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipGenerated(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if (skippy < 0) || (iNdEx+skippy) < 0 {
				return ErrInvalidLengthGenerated
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}

func skipGenerated(dAtA []byte) (n int, err error) {
	l := len(dAtA)
	iNdEx := 0
	depth := 0
	for iNdEx < l {
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, ErrIntOverflowGenerated
			}
			if iNdEx >= l {
				return 0, io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		wireType := int(wire & 0x7)
		switch wireType {
		case 0:
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowGenerated
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				iNdEx++
				if dAtA[iNdEx-1] < 0x80 {
					break
				}
			}
		case 1:
			iNdEx += 8
		case 2:
			var length int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowGenerated
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				length |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if length < 0 {
				return 0, ErrInvalidLengthGenerated
			}
			iNdEx += length
		case 3:
			depth++
		case 4:
			if depth == 0 {
				return 0, ErrUnexpectedEndOfGroupGenerated
			}
			depth--
		case 5:
			iNdEx += 4
		default:
			return 0, fmt.Errorf("proto: illegal wireType %d", wireType)
		}
		if iNdEx < 0 {
			return 0, ErrInvalidLengthGenerated
		}
		if depth == 0 {
			return iNdEx, nil
		}
	}
	return 0, io.ErrUnexpectedEOF
}

var (
	ErrInvalidLengthGenerated        = fmt.Errorf("proto: negative length found during unmarshaling")
	ErrIntOverflowGenerated          = fmt.Errorf("proto: integer overflow")
	ErrUnexpectedEndOfGroupGenerated = fmt.Errorf("proto: unexpected end of group")
)

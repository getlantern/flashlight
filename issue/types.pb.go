// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.3
// 	protoc        v5.29.3
// source: types.proto

package issue

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type Request_ISSUE_TYPE int32

const (
	Request_PAYMENT_FAIL         Request_ISSUE_TYPE = 0
	Request_CANNOT_LOGIN         Request_ISSUE_TYPE = 1
	Request_ALWAYS_SPINNING      Request_ISSUE_TYPE = 2
	Request_NO_ACCESS            Request_ISSUE_TYPE = 3
	Request_SLOW                 Request_ISSUE_TYPE = 4
	Request_CANNOT_LINK_DEVICE   Request_ISSUE_TYPE = 5
	Request_CRASHES              Request_ISSUE_TYPE = 6
	Request_CHAT_NOT_WORKING     Request_ISSUE_TYPE = 7
	Request_DISCOVER_NOT_WORKING Request_ISSUE_TYPE = 8
	Request_OTHER                Request_ISSUE_TYPE = 9
	Request_UPDATE_FAIL          Request_ISSUE_TYPE = 10
)

// Enum value maps for Request_ISSUE_TYPE.
var (
	Request_ISSUE_TYPE_name = map[int32]string{
		0:  "PAYMENT_FAIL",
		1:  "CANNOT_LOGIN",
		2:  "ALWAYS_SPINNING",
		3:  "NO_ACCESS",
		4:  "SLOW",
		5:  "CANNOT_LINK_DEVICE",
		6:  "CRASHES",
		7:  "CHAT_NOT_WORKING",
		8:  "DISCOVER_NOT_WORKING",
		9:  "OTHER",
		10: "UPDATE_FAIL",
	}
	Request_ISSUE_TYPE_value = map[string]int32{
		"PAYMENT_FAIL":         0,
		"CANNOT_LOGIN":         1,
		"ALWAYS_SPINNING":      2,
		"NO_ACCESS":            3,
		"SLOW":                 4,
		"CANNOT_LINK_DEVICE":   5,
		"CRASHES":              6,
		"CHAT_NOT_WORKING":     7,
		"DISCOVER_NOT_WORKING": 8,
		"OTHER":                9,
		"UPDATE_FAIL":          10,
	}
)

func (x Request_ISSUE_TYPE) Enum() *Request_ISSUE_TYPE {
	p := new(Request_ISSUE_TYPE)
	*p = x
	return p
}

func (x Request_ISSUE_TYPE) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (Request_ISSUE_TYPE) Descriptor() protoreflect.EnumDescriptor {
	return file_types_proto_enumTypes[0].Descriptor()
}

func (Request_ISSUE_TYPE) Type() protoreflect.EnumType {
	return &file_types_proto_enumTypes[0]
}

func (x Request_ISSUE_TYPE) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use Request_ISSUE_TYPE.Descriptor instead.
func (Request_ISSUE_TYPE) EnumDescriptor() ([]byte, []int) {
	return file_types_proto_rawDescGZIP(), []int{1, 0}
}

type Response struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Response) Reset() {
	*x = Response{}
	mi := &file_types_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Response) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Response) ProtoMessage() {}

func (x *Response) ProtoReflect() protoreflect.Message {
	mi := &file_types_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Response.ProtoReflect.Descriptor instead.
func (*Response) Descriptor() ([]byte, []int) {
	return file_types_proto_rawDescGZIP(), []int{0}
}

type Request struct {
	state             protoimpl.MessageState `protogen:"open.v1"`
	Type              Request_ISSUE_TYPE     `protobuf:"varint,1,opt,name=type,proto3,enum=issue.Request_ISSUE_TYPE" json:"type,omitempty"`
	CountryCode       string                 `protobuf:"bytes,2,opt,name=country_code,json=countryCode,proto3" json:"country_code,omitempty"`
	AppVersion        string                 `protobuf:"bytes,3,opt,name=app_version,json=appVersion,proto3" json:"app_version,omitempty"`
	SubscriptionLevel string                 `protobuf:"bytes,4,opt,name=subscription_level,json=subscriptionLevel,proto3" json:"subscription_level,omitempty"`
	Platform          string                 `protobuf:"bytes,5,opt,name=platform,proto3" json:"platform,omitempty"`
	Description       string                 `protobuf:"bytes,6,opt,name=description,proto3" json:"description,omitempty"`
	UserEmail         string                 `protobuf:"bytes,7,opt,name=user_email,json=userEmail,proto3" json:"user_email,omitempty"`
	DeviceId          string                 `protobuf:"bytes,8,opt,name=device_id,json=deviceId,proto3" json:"device_id,omitempty"`
	UserId            string                 `protobuf:"bytes,9,opt,name=user_id,json=userId,proto3" json:"user_id,omitempty"`
	ProToken          string                 `protobuf:"bytes,10,opt,name=pro_token,json=proToken,proto3" json:"pro_token,omitempty"`
	Device            string                 `protobuf:"bytes,11,opt,name=device,proto3" json:"device,omitempty"`
	Model             string                 `protobuf:"bytes,12,opt,name=model,proto3" json:"model,omitempty"`
	OsVersion         string                 `protobuf:"bytes,13,opt,name=os_version,json=osVersion,proto3" json:"os_version,omitempty"`
	Language          string                 `protobuf:"bytes,14,opt,name=language,proto3" json:"language,omitempty"`
	Attachments       []*Request_Attachment  `protobuf:"bytes,15,rep,name=attachments,proto3" json:"attachments,omitempty"`
	unknownFields     protoimpl.UnknownFields
	sizeCache         protoimpl.SizeCache
}

func (x *Request) Reset() {
	*x = Request{}
	mi := &file_types_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Request) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Request) ProtoMessage() {}

func (x *Request) ProtoReflect() protoreflect.Message {
	mi := &file_types_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Request.ProtoReflect.Descriptor instead.
func (*Request) Descriptor() ([]byte, []int) {
	return file_types_proto_rawDescGZIP(), []int{1}
}

func (x *Request) GetType() Request_ISSUE_TYPE {
	if x != nil {
		return x.Type
	}
	return Request_PAYMENT_FAIL
}

func (x *Request) GetCountryCode() string {
	if x != nil {
		return x.CountryCode
	}
	return ""
}

func (x *Request) GetAppVersion() string {
	if x != nil {
		return x.AppVersion
	}
	return ""
}

func (x *Request) GetSubscriptionLevel() string {
	if x != nil {
		return x.SubscriptionLevel
	}
	return ""
}

func (x *Request) GetPlatform() string {
	if x != nil {
		return x.Platform
	}
	return ""
}

func (x *Request) GetDescription() string {
	if x != nil {
		return x.Description
	}
	return ""
}

func (x *Request) GetUserEmail() string {
	if x != nil {
		return x.UserEmail
	}
	return ""
}

func (x *Request) GetDeviceId() string {
	if x != nil {
		return x.DeviceId
	}
	return ""
}

func (x *Request) GetUserId() string {
	if x != nil {
		return x.UserId
	}
	return ""
}

func (x *Request) GetProToken() string {
	if x != nil {
		return x.ProToken
	}
	return ""
}

func (x *Request) GetDevice() string {
	if x != nil {
		return x.Device
	}
	return ""
}

func (x *Request) GetModel() string {
	if x != nil {
		return x.Model
	}
	return ""
}

func (x *Request) GetOsVersion() string {
	if x != nil {
		return x.OsVersion
	}
	return ""
}

func (x *Request) GetLanguage() string {
	if x != nil {
		return x.Language
	}
	return ""
}

func (x *Request) GetAttachments() []*Request_Attachment {
	if x != nil {
		return x.Attachments
	}
	return nil
}

type Request_Attachment struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Type          string                 `protobuf:"bytes,1,opt,name=type,proto3" json:"type,omitempty"`
	Name          string                 `protobuf:"bytes,2,opt,name=name,proto3" json:"name,omitempty"`
	Content       []byte                 `protobuf:"bytes,3,opt,name=content,proto3" json:"content,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Request_Attachment) Reset() {
	*x = Request_Attachment{}
	mi := &file_types_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Request_Attachment) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Request_Attachment) ProtoMessage() {}

func (x *Request_Attachment) ProtoReflect() protoreflect.Message {
	mi := &file_types_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Request_Attachment.ProtoReflect.Descriptor instead.
func (*Request_Attachment) Descriptor() ([]byte, []int) {
	return file_types_proto_rawDescGZIP(), []int{1, 0}
}

func (x *Request_Attachment) GetType() string {
	if x != nil {
		return x.Type
	}
	return ""
}

func (x *Request_Attachment) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *Request_Attachment) GetContent() []byte {
	if x != nil {
		return x.Content
	}
	return nil
}

var File_types_proto protoreflect.FileDescriptor

var file_types_proto_rawDesc = []byte{
	0x0a, 0x0b, 0x74, 0x79, 0x70, 0x65, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x05, 0x69,
	0x73, 0x73, 0x75, 0x65, 0x22, 0x0a, 0x0a, 0x08, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65,
	0x22, 0xa3, 0x06, 0x0a, 0x07, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x2d, 0x0a, 0x04,
	0x74, 0x79, 0x70, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x19, 0x2e, 0x69, 0x73, 0x73,
	0x75, 0x65, 0x2e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x2e, 0x49, 0x53, 0x53, 0x55, 0x45,
	0x5f, 0x54, 0x59, 0x50, 0x45, 0x52, 0x04, 0x74, 0x79, 0x70, 0x65, 0x12, 0x21, 0x0a, 0x0c, 0x63,
	0x6f, 0x75, 0x6e, 0x74, 0x72, 0x79, 0x5f, 0x63, 0x6f, 0x64, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x0b, 0x63, 0x6f, 0x75, 0x6e, 0x74, 0x72, 0x79, 0x43, 0x6f, 0x64, 0x65, 0x12, 0x1f,
	0x0a, 0x0b, 0x61, 0x70, 0x70, 0x5f, 0x76, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x18, 0x03, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x0a, 0x61, 0x70, 0x70, 0x56, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x12,
	0x2d, 0x0a, 0x12, 0x73, 0x75, 0x62, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x5f,
	0x6c, 0x65, 0x76, 0x65, 0x6c, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x52, 0x11, 0x73, 0x75, 0x62,
	0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x4c, 0x65, 0x76, 0x65, 0x6c, 0x12, 0x1a,
	0x0a, 0x08, 0x70, 0x6c, 0x61, 0x74, 0x66, 0x6f, 0x72, 0x6d, 0x18, 0x05, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x08, 0x70, 0x6c, 0x61, 0x74, 0x66, 0x6f, 0x72, 0x6d, 0x12, 0x20, 0x0a, 0x0b, 0x64, 0x65,
	0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x18, 0x06, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x0b, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x1d, 0x0a, 0x0a,
	0x75, 0x73, 0x65, 0x72, 0x5f, 0x65, 0x6d, 0x61, 0x69, 0x6c, 0x18, 0x07, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x09, 0x75, 0x73, 0x65, 0x72, 0x45, 0x6d, 0x61, 0x69, 0x6c, 0x12, 0x1b, 0x0a, 0x09, 0x64,
	0x65, 0x76, 0x69, 0x63, 0x65, 0x5f, 0x69, 0x64, 0x18, 0x08, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08,
	0x64, 0x65, 0x76, 0x69, 0x63, 0x65, 0x49, 0x64, 0x12, 0x17, 0x0a, 0x07, 0x75, 0x73, 0x65, 0x72,
	0x5f, 0x69, 0x64, 0x18, 0x09, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06, 0x75, 0x73, 0x65, 0x72, 0x49,
	0x64, 0x12, 0x1b, 0x0a, 0x09, 0x70, 0x72, 0x6f, 0x5f, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x18, 0x0a,
	0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x70, 0x72, 0x6f, 0x54, 0x6f, 0x6b, 0x65, 0x6e, 0x12, 0x16,
	0x0a, 0x06, 0x64, 0x65, 0x76, 0x69, 0x63, 0x65, 0x18, 0x0b, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06,
	0x64, 0x65, 0x76, 0x69, 0x63, 0x65, 0x12, 0x14, 0x0a, 0x05, 0x6d, 0x6f, 0x64, 0x65, 0x6c, 0x18,
	0x0c, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x6d, 0x6f, 0x64, 0x65, 0x6c, 0x12, 0x1d, 0x0a, 0x0a,
	0x6f, 0x73, 0x5f, 0x76, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x18, 0x0d, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x09, 0x6f, 0x73, 0x56, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x12, 0x1a, 0x0a, 0x08, 0x6c,
	0x61, 0x6e, 0x67, 0x75, 0x61, 0x67, 0x65, 0x18, 0x0e, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x6c,
	0x61, 0x6e, 0x67, 0x75, 0x61, 0x67, 0x65, 0x12, 0x3b, 0x0a, 0x0b, 0x61, 0x74, 0x74, 0x61, 0x63,
	0x68, 0x6d, 0x65, 0x6e, 0x74, 0x73, 0x18, 0x0f, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x19, 0x2e, 0x69,
	0x73, 0x73, 0x75, 0x65, 0x2e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x2e, 0x41, 0x74, 0x74,
	0x61, 0x63, 0x68, 0x6d, 0x65, 0x6e, 0x74, 0x52, 0x0b, 0x61, 0x74, 0x74, 0x61, 0x63, 0x68, 0x6d,
	0x65, 0x6e, 0x74, 0x73, 0x1a, 0x4e, 0x0a, 0x0a, 0x41, 0x74, 0x74, 0x61, 0x63, 0x68, 0x6d, 0x65,
	0x6e, 0x74, 0x12, 0x12, 0x0a, 0x04, 0x74, 0x79, 0x70, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x04, 0x74, 0x79, 0x70, 0x65, 0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x02,
	0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x18, 0x0a, 0x07, 0x63, 0x6f,
	0x6e, 0x74, 0x65, 0x6e, 0x74, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x07, 0x63, 0x6f, 0x6e,
	0x74, 0x65, 0x6e, 0x74, 0x22, 0xcf, 0x01, 0x0a, 0x0a, 0x49, 0x53, 0x53, 0x55, 0x45, 0x5f, 0x54,
	0x59, 0x50, 0x45, 0x12, 0x10, 0x0a, 0x0c, 0x50, 0x41, 0x59, 0x4d, 0x45, 0x4e, 0x54, 0x5f, 0x46,
	0x41, 0x49, 0x4c, 0x10, 0x00, 0x12, 0x10, 0x0a, 0x0c, 0x43, 0x41, 0x4e, 0x4e, 0x4f, 0x54, 0x5f,
	0x4c, 0x4f, 0x47, 0x49, 0x4e, 0x10, 0x01, 0x12, 0x13, 0x0a, 0x0f, 0x41, 0x4c, 0x57, 0x41, 0x59,
	0x53, 0x5f, 0x53, 0x50, 0x49, 0x4e, 0x4e, 0x49, 0x4e, 0x47, 0x10, 0x02, 0x12, 0x0d, 0x0a, 0x09,
	0x4e, 0x4f, 0x5f, 0x41, 0x43, 0x43, 0x45, 0x53, 0x53, 0x10, 0x03, 0x12, 0x08, 0x0a, 0x04, 0x53,
	0x4c, 0x4f, 0x57, 0x10, 0x04, 0x12, 0x16, 0x0a, 0x12, 0x43, 0x41, 0x4e, 0x4e, 0x4f, 0x54, 0x5f,
	0x4c, 0x49, 0x4e, 0x4b, 0x5f, 0x44, 0x45, 0x56, 0x49, 0x43, 0x45, 0x10, 0x05, 0x12, 0x0b, 0x0a,
	0x07, 0x43, 0x52, 0x41, 0x53, 0x48, 0x45, 0x53, 0x10, 0x06, 0x12, 0x14, 0x0a, 0x10, 0x43, 0x48,
	0x41, 0x54, 0x5f, 0x4e, 0x4f, 0x54, 0x5f, 0x57, 0x4f, 0x52, 0x4b, 0x49, 0x4e, 0x47, 0x10, 0x07,
	0x12, 0x18, 0x0a, 0x14, 0x44, 0x49, 0x53, 0x43, 0x4f, 0x56, 0x45, 0x52, 0x5f, 0x4e, 0x4f, 0x54,
	0x5f, 0x57, 0x4f, 0x52, 0x4b, 0x49, 0x4e, 0x47, 0x10, 0x08, 0x12, 0x09, 0x0a, 0x05, 0x4f, 0x54,
	0x48, 0x45, 0x52, 0x10, 0x09, 0x12, 0x0f, 0x0a, 0x0b, 0x55, 0x50, 0x44, 0x41, 0x54, 0x45, 0x5f,
	0x46, 0x41, 0x49, 0x4c, 0x10, 0x0a, 0x42, 0x28, 0x5a, 0x26, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62,
	0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x67, 0x65, 0x74, 0x6c, 0x61, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x2f,
	0x66, 0x6c, 0x61, 0x73, 0x68, 0x6c, 0x69, 0x67, 0x68, 0x74, 0x2f, 0x69, 0x73, 0x73, 0x75, 0x65,
	0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_types_proto_rawDescOnce sync.Once
	file_types_proto_rawDescData = file_types_proto_rawDesc
)

func file_types_proto_rawDescGZIP() []byte {
	file_types_proto_rawDescOnce.Do(func() {
		file_types_proto_rawDescData = protoimpl.X.CompressGZIP(file_types_proto_rawDescData)
	})
	return file_types_proto_rawDescData
}

var file_types_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_types_proto_msgTypes = make([]protoimpl.MessageInfo, 3)
var file_types_proto_goTypes = []any{
	(Request_ISSUE_TYPE)(0),    // 0: issue.Request.ISSUE_TYPE
	(*Response)(nil),           // 1: issue.Response
	(*Request)(nil),            // 2: issue.Request
	(*Request_Attachment)(nil), // 3: issue.Request.Attachment
}
var file_types_proto_depIdxs = []int32{
	0, // 0: issue.Request.type:type_name -> issue.Request.ISSUE_TYPE
	3, // 1: issue.Request.attachments:type_name -> issue.Request.Attachment
	2, // [2:2] is the sub-list for method output_type
	2, // [2:2] is the sub-list for method input_type
	2, // [2:2] is the sub-list for extension type_name
	2, // [2:2] is the sub-list for extension extendee
	0, // [0:2] is the sub-list for field type_name
}

func init() { file_types_proto_init() }
func file_types_proto_init() {
	if File_types_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_types_proto_rawDesc,
			NumEnums:      1,
			NumMessages:   3,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_types_proto_goTypes,
		DependencyIndexes: file_types_proto_depIdxs,
		EnumInfos:         file_types_proto_enumTypes,
		MessageInfos:      file_types_proto_msgTypes,
	}.Build()
	File_types_proto = out.File
	file_types_proto_rawDesc = nil
	file_types_proto_goTypes = nil
	file_types_proto_depIdxs = nil
}

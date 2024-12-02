//
// Copied from lantern-cloud at a2ebb22f252f078477ab4bd0458a661089f82fea. DO NOT EDIT.
//

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.33.0
// 	protoc        v5.28.3
// source: apipb/types.proto

package apipb

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

// LegacyConnectConfig is the information required for a client to connect
// to a proxy, for clients which get their config from config-server (it's
// copied from there).
type LegacyConnectConfig struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Name                              string                             `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	Addr                              string                             `protobuf:"bytes,2,opt,name=addr,proto3" json:"addr,omitempty"`
	Cert                              string                             `protobuf:"bytes,3,opt,name=cert,proto3" json:"cert,omitempty"`
	AuthToken                         string                             `protobuf:"bytes,4,opt,name=auth_token,json=authToken,proto3" json:"auth_token,omitempty"`
	Trusted                           bool                               `protobuf:"varint,5,opt,name=trusted,proto3" json:"trusted,omitempty"`
	MaxPreconnect                     int32                              `protobuf:"varint,6,opt,name=max_preconnect,json=maxPreconnect,proto3" json:"max_preconnect,omitempty"`
	Bias                              int32                              `protobuf:"varint,7,opt,name=bias,proto3" json:"bias,omitempty"`
	PluggableTransport                string                             `protobuf:"bytes,8,opt,name=pluggable_transport,json=pluggableTransport,proto3" json:"pluggable_transport,omitempty"`
	PluggableTransportSettings        map[string]string                  `protobuf:"bytes,9,rep,name=pluggable_transport_settings,json=pluggableTransportSettings,proto3" json:"pluggable_transport_settings,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
	ENHTTPURL                         string                             `protobuf:"bytes,10,opt,name=ENHTTPURL,proto3" json:"ENHTTPURL,omitempty"`
	TLSDesktopOrderedCipherSuiteNames []string                           `protobuf:"bytes,11,rep,name=TLSDesktop_ordered_cipher_suite_names,json=TLSDesktopOrderedCipherSuiteNames,proto3" json:"TLSDesktop_ordered_cipher_suite_names,omitempty"`
	TLSMobileOrderedCipherSuiteNames  []string                           `protobuf:"bytes,12,rep,name=TLSMobile_ordered_cipher_suite_names,json=TLSMobileOrderedCipherSuiteNames,proto3" json:"TLSMobile_ordered_cipher_suite_names,omitempty"`
	TLSServerNameIndicator            string                             `protobuf:"bytes,13,opt,name=TLSServer_name_indicator,json=TLSServerNameIndicator,proto3" json:"TLSServer_name_indicator,omitempty"`
	TLSClientSessionCacheSize         int32                              `protobuf:"varint,14,opt,name=TLSClient_session_cache_size,json=TLSClientSessionCacheSize,proto3" json:"TLSClient_session_cache_size,omitempty"`
	TLSClientHelloID                  string                             `protobuf:"bytes,15,opt,name=TLSClient_helloID,json=TLSClientHelloID,proto3" json:"TLSClient_helloID,omitempty"`
	TLSClientHelloSplitting           bool                               `protobuf:"varint,16,opt,name=TLSClient_hello_splitting,json=TLSClientHelloSplitting,proto3" json:"TLSClient_hello_splitting,omitempty"`
	TLSClientSessionState             string                             `protobuf:"bytes,17,opt,name=TLSClient_session_state,json=TLSClientSessionState,proto3" json:"TLSClient_session_state,omitempty"`
	Location                          *LegacyConnectConfig_ProxyLocation `protobuf:"bytes,18,opt,name=location,proto3" json:"location,omitempty"`
	MultipathEndpoint                 string                             `protobuf:"bytes,19,opt,name=multipath_endpoint,json=multipathEndpoint,proto3" json:"multipath_endpoint,omitempty"`
	MultiplexedAddr                   string                             `protobuf:"bytes,20,opt,name=multiplexed_addr,json=multiplexedAddr,proto3" json:"multiplexed_addr,omitempty"`
	MultiplexedPhysicalConns          int32                              `protobuf:"varint,21,opt,name=multiplexed_physical_conns,json=multiplexedPhysicalConns,proto3" json:"multiplexed_physical_conns,omitempty"`
	MultiplexedProtocol               string                             `protobuf:"bytes,22,opt,name=multiplexed_protocol,json=multiplexedProtocol,proto3" json:"multiplexed_protocol,omitempty"`
	MultiplexedSettings               map[string]string                  `protobuf:"bytes,23,rep,name=multiplexed_settings,json=multiplexedSettings,proto3" json:"multiplexed_settings,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
	Track                             string                             `protobuf:"bytes,24,opt,name=track,proto3" json:"track,omitempty"`
	Region                            string                             `protobuf:"bytes,25,opt,name=region,proto3" json:"region,omitempty"`
}

func (x *LegacyConnectConfig) Reset() {
	*x = LegacyConnectConfig{}
	if protoimpl.UnsafeEnabled {
		mi := &file_apipb_types_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *LegacyConnectConfig) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*LegacyConnectConfig) ProtoMessage() {}

func (x *LegacyConnectConfig) ProtoReflect() protoreflect.Message {
	mi := &file_apipb_types_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use LegacyConnectConfig.ProtoReflect.Descriptor instead.
func (*LegacyConnectConfig) Descriptor() ([]byte, []int) {
	return file_apipb_types_proto_rawDescGZIP(), []int{0}
}

func (x *LegacyConnectConfig) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *LegacyConnectConfig) GetAddr() string {
	if x != nil {
		return x.Addr
	}
	return ""
}

func (x *LegacyConnectConfig) GetCert() string {
	if x != nil {
		return x.Cert
	}
	return ""
}

func (x *LegacyConnectConfig) GetAuthToken() string {
	if x != nil {
		return x.AuthToken
	}
	return ""
}

func (x *LegacyConnectConfig) GetTrusted() bool {
	if x != nil {
		return x.Trusted
	}
	return false
}

func (x *LegacyConnectConfig) GetMaxPreconnect() int32 {
	if x != nil {
		return x.MaxPreconnect
	}
	return 0
}

func (x *LegacyConnectConfig) GetBias() int32 {
	if x != nil {
		return x.Bias
	}
	return 0
}

func (x *LegacyConnectConfig) GetPluggableTransport() string {
	if x != nil {
		return x.PluggableTransport
	}
	return ""
}

func (x *LegacyConnectConfig) GetPluggableTransportSettings() map[string]string {
	if x != nil {
		return x.PluggableTransportSettings
	}
	return nil
}

func (x *LegacyConnectConfig) GetENHTTPURL() string {
	if x != nil {
		return x.ENHTTPURL
	}
	return ""
}

func (x *LegacyConnectConfig) GetTLSDesktopOrderedCipherSuiteNames() []string {
	if x != nil {
		return x.TLSDesktopOrderedCipherSuiteNames
	}
	return nil
}

func (x *LegacyConnectConfig) GetTLSMobileOrderedCipherSuiteNames() []string {
	if x != nil {
		return x.TLSMobileOrderedCipherSuiteNames
	}
	return nil
}

func (x *LegacyConnectConfig) GetTLSServerNameIndicator() string {
	if x != nil {
		return x.TLSServerNameIndicator
	}
	return ""
}

func (x *LegacyConnectConfig) GetTLSClientSessionCacheSize() int32 {
	if x != nil {
		return x.TLSClientSessionCacheSize
	}
	return 0
}

func (x *LegacyConnectConfig) GetTLSClientHelloID() string {
	if x != nil {
		return x.TLSClientHelloID
	}
	return ""
}

func (x *LegacyConnectConfig) GetTLSClientHelloSplitting() bool {
	if x != nil {
		return x.TLSClientHelloSplitting
	}
	return false
}

func (x *LegacyConnectConfig) GetTLSClientSessionState() string {
	if x != nil {
		return x.TLSClientSessionState
	}
	return ""
}

func (x *LegacyConnectConfig) GetLocation() *LegacyConnectConfig_ProxyLocation {
	if x != nil {
		return x.Location
	}
	return nil
}

func (x *LegacyConnectConfig) GetMultipathEndpoint() string {
	if x != nil {
		return x.MultipathEndpoint
	}
	return ""
}

func (x *LegacyConnectConfig) GetMultiplexedAddr() string {
	if x != nil {
		return x.MultiplexedAddr
	}
	return ""
}

func (x *LegacyConnectConfig) GetMultiplexedPhysicalConns() int32 {
	if x != nil {
		return x.MultiplexedPhysicalConns
	}
	return 0
}

func (x *LegacyConnectConfig) GetMultiplexedProtocol() string {
	if x != nil {
		return x.MultiplexedProtocol
	}
	return ""
}

func (x *LegacyConnectConfig) GetMultiplexedSettings() map[string]string {
	if x != nil {
		return x.MultiplexedSettings
	}
	return nil
}

func (x *LegacyConnectConfig) GetTrack() string {
	if x != nil {
		return x.Track
	}
	return ""
}

func (x *LegacyConnectConfig) GetRegion() string {
	if x != nil {
		return x.Region
	}
	return ""
}

type BypassRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Config *LegacyConnectConfig `protobuf:"bytes,1,opt,name=config,proto3" json:"config,omitempty"`
	// Version is a version specifier denoting the version of bypass used on the client. It is not
	// necessary to update this value on every change to bypass; this should only be updated when the
	// backend needs to make decisions unique to a new version of bypass.
	Version int32 `protobuf:"varint,2,opt,name=version,proto3" json:"version,omitempty"`
}

func (x *BypassRequest) Reset() {
	*x = BypassRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_apipb_types_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *BypassRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*BypassRequest) ProtoMessage() {}

func (x *BypassRequest) ProtoReflect() protoreflect.Message {
	mi := &file_apipb_types_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use BypassRequest.ProtoReflect.Descriptor instead.
func (*BypassRequest) Descriptor() ([]byte, []int) {
	return file_apipb_types_proto_rawDescGZIP(), []int{1}
}

func (x *BypassRequest) GetConfig() *LegacyConnectConfig {
	if x != nil {
		return x.Config
	}
	return nil
}

func (x *BypassRequest) GetVersion() int32 {
	if x != nil {
		return x.Version
	}
	return 0
}

type BypassResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Random string `protobuf:"bytes,1,opt,name=random,proto3" json:"random,omitempty"`
}

func (x *BypassResponse) Reset() {
	*x = BypassResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_apipb_types_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *BypassResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*BypassResponse) ProtoMessage() {}

func (x *BypassResponse) ProtoReflect() protoreflect.Message {
	mi := &file_apipb_types_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use BypassResponse.ProtoReflect.Descriptor instead.
func (*BypassResponse) Descriptor() ([]byte, []int) {
	return file_apipb_types_proto_rawDescGZIP(), []int{2}
}

func (x *BypassResponse) GetRandom() string {
	if x != nil {
		return x.Random
	}
	return ""
}

type LegacyConnectConfig_ProxyLocation struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	City        string  `protobuf:"bytes,1,opt,name=city,proto3" json:"city,omitempty"`
	Country     string  `protobuf:"bytes,2,opt,name=country,proto3" json:"country,omitempty"`
	CountryCode string  `protobuf:"bytes,3,opt,name=country_code,json=countryCode,proto3" json:"country_code,omitempty"`
	Latitude    float32 `protobuf:"fixed32,4,opt,name=latitude,proto3" json:"latitude,omitempty"`
	Longitude   float32 `protobuf:"fixed32,5,opt,name=longitude,proto3" json:"longitude,omitempty"`
}

func (x *LegacyConnectConfig_ProxyLocation) Reset() {
	*x = LegacyConnectConfig_ProxyLocation{}
	if protoimpl.UnsafeEnabled {
		mi := &file_apipb_types_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *LegacyConnectConfig_ProxyLocation) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*LegacyConnectConfig_ProxyLocation) ProtoMessage() {}

func (x *LegacyConnectConfig_ProxyLocation) ProtoReflect() protoreflect.Message {
	mi := &file_apipb_types_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use LegacyConnectConfig_ProxyLocation.ProtoReflect.Descriptor instead.
func (*LegacyConnectConfig_ProxyLocation) Descriptor() ([]byte, []int) {
	return file_apipb_types_proto_rawDescGZIP(), []int{0, 0}
}

func (x *LegacyConnectConfig_ProxyLocation) GetCity() string {
	if x != nil {
		return x.City
	}
	return ""
}

func (x *LegacyConnectConfig_ProxyLocation) GetCountry() string {
	if x != nil {
		return x.Country
	}
	return ""
}

func (x *LegacyConnectConfig_ProxyLocation) GetCountryCode() string {
	if x != nil {
		return x.CountryCode
	}
	return ""
}

func (x *LegacyConnectConfig_ProxyLocation) GetLatitude() float32 {
	if x != nil {
		return x.Latitude
	}
	return 0
}

func (x *LegacyConnectConfig_ProxyLocation) GetLongitude() float32 {
	if x != nil {
		return x.Longitude
	}
	return 0
}

var File_apipb_types_proto protoreflect.FileDescriptor

var file_apipb_types_proto_rawDesc = []byte{
	0x0a, 0x11, 0x61, 0x70, 0x69, 0x70, 0x62, 0x2f, 0x74, 0x79, 0x70, 0x65, 0x73, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x22, 0x99, 0x0c, 0x0a, 0x13, 0x4c, 0x65, 0x67, 0x61, 0x63, 0x79, 0x43, 0x6f,
	0x6e, 0x6e, 0x65, 0x63, 0x74, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x12, 0x12, 0x0a, 0x04, 0x6e,
	0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x12,
	0x12, 0x0a, 0x04, 0x61, 0x64, 0x64, 0x72, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x61,
	0x64, 0x64, 0x72, 0x12, 0x12, 0x0a, 0x04, 0x63, 0x65, 0x72, 0x74, 0x18, 0x03, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x04, 0x63, 0x65, 0x72, 0x74, 0x12, 0x1d, 0x0a, 0x0a, 0x61, 0x75, 0x74, 0x68, 0x5f,
	0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x52, 0x09, 0x61, 0x75, 0x74,
	0x68, 0x54, 0x6f, 0x6b, 0x65, 0x6e, 0x12, 0x18, 0x0a, 0x07, 0x74, 0x72, 0x75, 0x73, 0x74, 0x65,
	0x64, 0x18, 0x05, 0x20, 0x01, 0x28, 0x08, 0x52, 0x07, 0x74, 0x72, 0x75, 0x73, 0x74, 0x65, 0x64,
	0x12, 0x25, 0x0a, 0x0e, 0x6d, 0x61, 0x78, 0x5f, 0x70, 0x72, 0x65, 0x63, 0x6f, 0x6e, 0x6e, 0x65,
	0x63, 0x74, 0x18, 0x06, 0x20, 0x01, 0x28, 0x05, 0x52, 0x0d, 0x6d, 0x61, 0x78, 0x50, 0x72, 0x65,
	0x63, 0x6f, 0x6e, 0x6e, 0x65, 0x63, 0x74, 0x12, 0x12, 0x0a, 0x04, 0x62, 0x69, 0x61, 0x73, 0x18,
	0x07, 0x20, 0x01, 0x28, 0x05, 0x52, 0x04, 0x62, 0x69, 0x61, 0x73, 0x12, 0x2f, 0x0a, 0x13, 0x70,
	0x6c, 0x75, 0x67, 0x67, 0x61, 0x62, 0x6c, 0x65, 0x5f, 0x74, 0x72, 0x61, 0x6e, 0x73, 0x70, 0x6f,
	0x72, 0x74, 0x18, 0x08, 0x20, 0x01, 0x28, 0x09, 0x52, 0x12, 0x70, 0x6c, 0x75, 0x67, 0x67, 0x61,
	0x62, 0x6c, 0x65, 0x54, 0x72, 0x61, 0x6e, 0x73, 0x70, 0x6f, 0x72, 0x74, 0x12, 0x76, 0x0a, 0x1c,
	0x70, 0x6c, 0x75, 0x67, 0x67, 0x61, 0x62, 0x6c, 0x65, 0x5f, 0x74, 0x72, 0x61, 0x6e, 0x73, 0x70,
	0x6f, 0x72, 0x74, 0x5f, 0x73, 0x65, 0x74, 0x74, 0x69, 0x6e, 0x67, 0x73, 0x18, 0x09, 0x20, 0x03,
	0x28, 0x0b, 0x32, 0x34, 0x2e, 0x4c, 0x65, 0x67, 0x61, 0x63, 0x79, 0x43, 0x6f, 0x6e, 0x6e, 0x65,
	0x63, 0x74, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x2e, 0x50, 0x6c, 0x75, 0x67, 0x67, 0x61, 0x62,
	0x6c, 0x65, 0x54, 0x72, 0x61, 0x6e, 0x73, 0x70, 0x6f, 0x72, 0x74, 0x53, 0x65, 0x74, 0x74, 0x69,
	0x6e, 0x67, 0x73, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x52, 0x1a, 0x70, 0x6c, 0x75, 0x67, 0x67, 0x61,
	0x62, 0x6c, 0x65, 0x54, 0x72, 0x61, 0x6e, 0x73, 0x70, 0x6f, 0x72, 0x74, 0x53, 0x65, 0x74, 0x74,
	0x69, 0x6e, 0x67, 0x73, 0x12, 0x1c, 0x0a, 0x09, 0x45, 0x4e, 0x48, 0x54, 0x54, 0x50, 0x55, 0x52,
	0x4c, 0x18, 0x0a, 0x20, 0x01, 0x28, 0x09, 0x52, 0x09, 0x45, 0x4e, 0x48, 0x54, 0x54, 0x50, 0x55,
	0x52, 0x4c, 0x12, 0x50, 0x0a, 0x25, 0x54, 0x4c, 0x53, 0x44, 0x65, 0x73, 0x6b, 0x74, 0x6f, 0x70,
	0x5f, 0x6f, 0x72, 0x64, 0x65, 0x72, 0x65, 0x64, 0x5f, 0x63, 0x69, 0x70, 0x68, 0x65, 0x72, 0x5f,
	0x73, 0x75, 0x69, 0x74, 0x65, 0x5f, 0x6e, 0x61, 0x6d, 0x65, 0x73, 0x18, 0x0b, 0x20, 0x03, 0x28,
	0x09, 0x52, 0x21, 0x54, 0x4c, 0x53, 0x44, 0x65, 0x73, 0x6b, 0x74, 0x6f, 0x70, 0x4f, 0x72, 0x64,
	0x65, 0x72, 0x65, 0x64, 0x43, 0x69, 0x70, 0x68, 0x65, 0x72, 0x53, 0x75, 0x69, 0x74, 0x65, 0x4e,
	0x61, 0x6d, 0x65, 0x73, 0x12, 0x4e, 0x0a, 0x24, 0x54, 0x4c, 0x53, 0x4d, 0x6f, 0x62, 0x69, 0x6c,
	0x65, 0x5f, 0x6f, 0x72, 0x64, 0x65, 0x72, 0x65, 0x64, 0x5f, 0x63, 0x69, 0x70, 0x68, 0x65, 0x72,
	0x5f, 0x73, 0x75, 0x69, 0x74, 0x65, 0x5f, 0x6e, 0x61, 0x6d, 0x65, 0x73, 0x18, 0x0c, 0x20, 0x03,
	0x28, 0x09, 0x52, 0x20, 0x54, 0x4c, 0x53, 0x4d, 0x6f, 0x62, 0x69, 0x6c, 0x65, 0x4f, 0x72, 0x64,
	0x65, 0x72, 0x65, 0x64, 0x43, 0x69, 0x70, 0x68, 0x65, 0x72, 0x53, 0x75, 0x69, 0x74, 0x65, 0x4e,
	0x61, 0x6d, 0x65, 0x73, 0x12, 0x38, 0x0a, 0x18, 0x54, 0x4c, 0x53, 0x53, 0x65, 0x72, 0x76, 0x65,
	0x72, 0x5f, 0x6e, 0x61, 0x6d, 0x65, 0x5f, 0x69, 0x6e, 0x64, 0x69, 0x63, 0x61, 0x74, 0x6f, 0x72,
	0x18, 0x0d, 0x20, 0x01, 0x28, 0x09, 0x52, 0x16, 0x54, 0x4c, 0x53, 0x53, 0x65, 0x72, 0x76, 0x65,
	0x72, 0x4e, 0x61, 0x6d, 0x65, 0x49, 0x6e, 0x64, 0x69, 0x63, 0x61, 0x74, 0x6f, 0x72, 0x12, 0x3f,
	0x0a, 0x1c, 0x54, 0x4c, 0x53, 0x43, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x5f, 0x73, 0x65, 0x73, 0x73,
	0x69, 0x6f, 0x6e, 0x5f, 0x63, 0x61, 0x63, 0x68, 0x65, 0x5f, 0x73, 0x69, 0x7a, 0x65, 0x18, 0x0e,
	0x20, 0x01, 0x28, 0x05, 0x52, 0x19, 0x54, 0x4c, 0x53, 0x43, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x53,
	0x65, 0x73, 0x73, 0x69, 0x6f, 0x6e, 0x43, 0x61, 0x63, 0x68, 0x65, 0x53, 0x69, 0x7a, 0x65, 0x12,
	0x2b, 0x0a, 0x11, 0x54, 0x4c, 0x53, 0x43, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x5f, 0x68, 0x65, 0x6c,
	0x6c, 0x6f, 0x49, 0x44, 0x18, 0x0f, 0x20, 0x01, 0x28, 0x09, 0x52, 0x10, 0x54, 0x4c, 0x53, 0x43,
	0x6c, 0x69, 0x65, 0x6e, 0x74, 0x48, 0x65, 0x6c, 0x6c, 0x6f, 0x49, 0x44, 0x12, 0x3a, 0x0a, 0x19,
	0x54, 0x4c, 0x53, 0x43, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x5f, 0x68, 0x65, 0x6c, 0x6c, 0x6f, 0x5f,
	0x73, 0x70, 0x6c, 0x69, 0x74, 0x74, 0x69, 0x6e, 0x67, 0x18, 0x10, 0x20, 0x01, 0x28, 0x08, 0x52,
	0x17, 0x54, 0x4c, 0x53, 0x43, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x48, 0x65, 0x6c, 0x6c, 0x6f, 0x53,
	0x70, 0x6c, 0x69, 0x74, 0x74, 0x69, 0x6e, 0x67, 0x12, 0x36, 0x0a, 0x17, 0x54, 0x4c, 0x53, 0x43,
	0x6c, 0x69, 0x65, 0x6e, 0x74, 0x5f, 0x73, 0x65, 0x73, 0x73, 0x69, 0x6f, 0x6e, 0x5f, 0x73, 0x74,
	0x61, 0x74, 0x65, 0x18, 0x11, 0x20, 0x01, 0x28, 0x09, 0x52, 0x15, 0x54, 0x4c, 0x53, 0x43, 0x6c,
	0x69, 0x65, 0x6e, 0x74, 0x53, 0x65, 0x73, 0x73, 0x69, 0x6f, 0x6e, 0x53, 0x74, 0x61, 0x74, 0x65,
	0x12, 0x3e, 0x0a, 0x08, 0x6c, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x18, 0x12, 0x20, 0x01,
	0x28, 0x0b, 0x32, 0x22, 0x2e, 0x4c, 0x65, 0x67, 0x61, 0x63, 0x79, 0x43, 0x6f, 0x6e, 0x6e, 0x65,
	0x63, 0x74, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x2e, 0x50, 0x72, 0x6f, 0x78, 0x79, 0x4c, 0x6f,
	0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x08, 0x6c, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e,
	0x12, 0x2d, 0x0a, 0x12, 0x6d, 0x75, 0x6c, 0x74, 0x69, 0x70, 0x61, 0x74, 0x68, 0x5f, 0x65, 0x6e,
	0x64, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x18, 0x13, 0x20, 0x01, 0x28, 0x09, 0x52, 0x11, 0x6d, 0x75,
	0x6c, 0x74, 0x69, 0x70, 0x61, 0x74, 0x68, 0x45, 0x6e, 0x64, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x12,
	0x29, 0x0a, 0x10, 0x6d, 0x75, 0x6c, 0x74, 0x69, 0x70, 0x6c, 0x65, 0x78, 0x65, 0x64, 0x5f, 0x61,
	0x64, 0x64, 0x72, 0x18, 0x14, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0f, 0x6d, 0x75, 0x6c, 0x74, 0x69,
	0x70, 0x6c, 0x65, 0x78, 0x65, 0x64, 0x41, 0x64, 0x64, 0x72, 0x12, 0x3c, 0x0a, 0x1a, 0x6d, 0x75,
	0x6c, 0x74, 0x69, 0x70, 0x6c, 0x65, 0x78, 0x65, 0x64, 0x5f, 0x70, 0x68, 0x79, 0x73, 0x69, 0x63,
	0x61, 0x6c, 0x5f, 0x63, 0x6f, 0x6e, 0x6e, 0x73, 0x18, 0x15, 0x20, 0x01, 0x28, 0x05, 0x52, 0x18,
	0x6d, 0x75, 0x6c, 0x74, 0x69, 0x70, 0x6c, 0x65, 0x78, 0x65, 0x64, 0x50, 0x68, 0x79, 0x73, 0x69,
	0x63, 0x61, 0x6c, 0x43, 0x6f, 0x6e, 0x6e, 0x73, 0x12, 0x31, 0x0a, 0x14, 0x6d, 0x75, 0x6c, 0x74,
	0x69, 0x70, 0x6c, 0x65, 0x78, 0x65, 0x64, 0x5f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c,
	0x18, 0x16, 0x20, 0x01, 0x28, 0x09, 0x52, 0x13, 0x6d, 0x75, 0x6c, 0x74, 0x69, 0x70, 0x6c, 0x65,
	0x78, 0x65, 0x64, 0x50, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x12, 0x60, 0x0a, 0x14, 0x6d,
	0x75, 0x6c, 0x74, 0x69, 0x70, 0x6c, 0x65, 0x78, 0x65, 0x64, 0x5f, 0x73, 0x65, 0x74, 0x74, 0x69,
	0x6e, 0x67, 0x73, 0x18, 0x17, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x2d, 0x2e, 0x4c, 0x65, 0x67, 0x61,
	0x63, 0x79, 0x43, 0x6f, 0x6e, 0x6e, 0x65, 0x63, 0x74, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x2e,
	0x4d, 0x75, 0x6c, 0x74, 0x69, 0x70, 0x6c, 0x65, 0x78, 0x65, 0x64, 0x53, 0x65, 0x74, 0x74, 0x69,
	0x6e, 0x67, 0x73, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x52, 0x13, 0x6d, 0x75, 0x6c, 0x74, 0x69, 0x70,
	0x6c, 0x65, 0x78, 0x65, 0x64, 0x53, 0x65, 0x74, 0x74, 0x69, 0x6e, 0x67, 0x73, 0x12, 0x14, 0x0a,
	0x05, 0x74, 0x72, 0x61, 0x63, 0x6b, 0x18, 0x18, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x74, 0x72,
	0x61, 0x63, 0x6b, 0x12, 0x16, 0x0a, 0x06, 0x72, 0x65, 0x67, 0x69, 0x6f, 0x6e, 0x18, 0x19, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x06, 0x72, 0x65, 0x67, 0x69, 0x6f, 0x6e, 0x1a, 0x9a, 0x01, 0x0a, 0x0d,
	0x50, 0x72, 0x6f, 0x78, 0x79, 0x4c, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x12, 0x0a,
	0x04, 0x63, 0x69, 0x74, 0x79, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x63, 0x69, 0x74,
	0x79, 0x12, 0x18, 0x0a, 0x07, 0x63, 0x6f, 0x75, 0x6e, 0x74, 0x72, 0x79, 0x18, 0x02, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x07, 0x63, 0x6f, 0x75, 0x6e, 0x74, 0x72, 0x79, 0x12, 0x21, 0x0a, 0x0c, 0x63,
	0x6f, 0x75, 0x6e, 0x74, 0x72, 0x79, 0x5f, 0x63, 0x6f, 0x64, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x0b, 0x63, 0x6f, 0x75, 0x6e, 0x74, 0x72, 0x79, 0x43, 0x6f, 0x64, 0x65, 0x12, 0x1a,
	0x0a, 0x08, 0x6c, 0x61, 0x74, 0x69, 0x74, 0x75, 0x64, 0x65, 0x18, 0x04, 0x20, 0x01, 0x28, 0x02,
	0x52, 0x08, 0x6c, 0x61, 0x74, 0x69, 0x74, 0x75, 0x64, 0x65, 0x12, 0x1c, 0x0a, 0x09, 0x6c, 0x6f,
	0x6e, 0x67, 0x69, 0x74, 0x75, 0x64, 0x65, 0x18, 0x05, 0x20, 0x01, 0x28, 0x02, 0x52, 0x09, 0x6c,
	0x6f, 0x6e, 0x67, 0x69, 0x74, 0x75, 0x64, 0x65, 0x1a, 0x4d, 0x0a, 0x1f, 0x50, 0x6c, 0x75, 0x67,
	0x67, 0x61, 0x62, 0x6c, 0x65, 0x54, 0x72, 0x61, 0x6e, 0x73, 0x70, 0x6f, 0x72, 0x74, 0x53, 0x65,
	0x74, 0x74, 0x69, 0x6e, 0x67, 0x73, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x12, 0x10, 0x0a, 0x03, 0x6b,
	0x65, 0x79, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x03, 0x6b, 0x65, 0x79, 0x12, 0x14, 0x0a,
	0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x76, 0x61,
	0x6c, 0x75, 0x65, 0x3a, 0x02, 0x38, 0x01, 0x1a, 0x46, 0x0a, 0x18, 0x4d, 0x75, 0x6c, 0x74, 0x69,
	0x70, 0x6c, 0x65, 0x78, 0x65, 0x64, 0x53, 0x65, 0x74, 0x74, 0x69, 0x6e, 0x67, 0x73, 0x45, 0x6e,
	0x74, 0x72, 0x79, 0x12, 0x10, 0x0a, 0x03, 0x6b, 0x65, 0x79, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x03, 0x6b, 0x65, 0x79, 0x12, 0x14, 0x0a, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x18, 0x02,
	0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x3a, 0x02, 0x38, 0x01, 0x22,
	0x57, 0x0a, 0x0d, 0x42, 0x79, 0x70, 0x61, 0x73, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74,
	0x12, 0x2c, 0x0a, 0x06, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b,
	0x32, 0x14, 0x2e, 0x4c, 0x65, 0x67, 0x61, 0x63, 0x79, 0x43, 0x6f, 0x6e, 0x6e, 0x65, 0x63, 0x74,
	0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x52, 0x06, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x12, 0x18,
	0x0a, 0x07, 0x76, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x18, 0x02, 0x20, 0x01, 0x28, 0x05, 0x52,
	0x07, 0x76, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x22, 0x28, 0x0a, 0x0e, 0x42, 0x79, 0x70, 0x61,
	0x73, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x16, 0x0a, 0x06, 0x72, 0x61,
	0x6e, 0x64, 0x6f, 0x6d, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06, 0x72, 0x61, 0x6e, 0x64,
	0x6f, 0x6d, 0x42, 0x28, 0x5a, 0x26, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d,
	0x2f, 0x67, 0x65, 0x74, 0x6c, 0x61, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x2f, 0x66, 0x6c, 0x61, 0x73,
	0x68, 0x6c, 0x69, 0x67, 0x68, 0x74, 0x2f, 0x61, 0x70, 0x69, 0x70, 0x62, 0x62, 0x06, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_apipb_types_proto_rawDescOnce sync.Once
	file_apipb_types_proto_rawDescData = file_apipb_types_proto_rawDesc
)

func file_apipb_types_proto_rawDescGZIP() []byte {
	file_apipb_types_proto_rawDescOnce.Do(func() {
		file_apipb_types_proto_rawDescData = protoimpl.X.CompressGZIP(file_apipb_types_proto_rawDescData)
	})
	return file_apipb_types_proto_rawDescData
}

var file_apipb_types_proto_msgTypes = make([]protoimpl.MessageInfo, 6)
var file_apipb_types_proto_goTypes = []interface{}{
	(*LegacyConnectConfig)(nil),               // 0: LegacyConnectConfig
	(*BypassRequest)(nil),                     // 1: BypassRequest
	(*BypassResponse)(nil),                    // 2: BypassResponse
	(*LegacyConnectConfig_ProxyLocation)(nil), // 3: LegacyConnectConfig.ProxyLocation
	nil, // 4: LegacyConnectConfig.PluggableTransportSettingsEntry
	nil, // 5: LegacyConnectConfig.MultiplexedSettingsEntry
}
var file_apipb_types_proto_depIdxs = []int32{
	4, // 0: LegacyConnectConfig.pluggable_transport_settings:type_name -> LegacyConnectConfig.PluggableTransportSettingsEntry
	3, // 1: LegacyConnectConfig.location:type_name -> LegacyConnectConfig.ProxyLocation
	5, // 2: LegacyConnectConfig.multiplexed_settings:type_name -> LegacyConnectConfig.MultiplexedSettingsEntry
	0, // 3: BypassRequest.config:type_name -> LegacyConnectConfig
	4, // [4:4] is the sub-list for method output_type
	4, // [4:4] is the sub-list for method input_type
	4, // [4:4] is the sub-list for extension type_name
	4, // [4:4] is the sub-list for extension extendee
	0, // [0:4] is the sub-list for field type_name
}

func init() { file_apipb_types_proto_init() }
func file_apipb_types_proto_init() {
	if File_apipb_types_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_apipb_types_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*LegacyConnectConfig); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_apipb_types_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*BypassRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_apipb_types_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*BypassResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_apipb_types_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*LegacyConnectConfig_ProxyLocation); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_apipb_types_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   6,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_apipb_types_proto_goTypes,
		DependencyIndexes: file_apipb_types_proto_depIdxs,
		MessageInfos:      file_apipb_types_proto_msgTypes,
	}.Build()
	File_apipb_types_proto = out.File
	file_apipb_types_proto_rawDesc = nil
	file_apipb_types_proto_goTypes = nil
	file_apipb_types_proto_depIdxs = nil
}

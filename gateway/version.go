package gateway

import (
	"strings"

	cmpp "github.com/bigwhite/gocmpp"
)

// ParseCmppVersion 解析配置中的 CMPP 协议版本字符串，返回 gocmpp 版本常量。
// 支持 "2.0" / "20" / "v2.0"（不区分大小写），其他值（含空字符串）默认返回 V30。
func ParseCmppVersion(s string) cmpp.Type {
	v := strings.ToLower(strings.TrimSpace(s))
	v = strings.TrimPrefix(v, "v")
	if v == "2.0" || v == "20" {
		return cmpp.V20
	}
	return cmpp.V30
}

// IsV20 判断给定的 gocmpp 版本常量是否为 CMPP 2.0
func IsV20(typ cmpp.Type) bool {
	return typ == cmpp.V20
}

// buildSubmitReqPkt 根据配置的协议版本构造短信提交请求包。
// CMPP 2.0 与 3.0 的包体字段有差异（2.0 没有 FeeTerminalType/DestTerminalType/LinkId，
// 使用 Reserve 字段），gocmpp 已分别实现，此处按版本选择正确的包类型。
func buildSubmitReqPkt(cfg *Config, message SmsMes, srcId string) cmpp.Packer {
	if IsV20(ParseCmppVersion(cfg.CmppVersion)) {
		return &cmpp.Cmpp2SubmitReqPkt{
			PkTotal:            1,
			PkNumber:           1,
			RegisteredDelivery: 0,
			MsgLevel:           1,
			ServiceId:          cfg.ServiceId,
			FeeUserType:        0,
			FeeTerminalId:      "",
			MsgFmt:             0,
			MsgSrc:             cfg.User, // MsgSrc应该是企业代码，即登录用户名
			FeeType:            "01",
			FeeCode:            "000000",
			ValidTime:          "",
			AtTime:             "",
			SrcId:              srcId,
			DestUsrTl:          1,
			DestTerminalId:     []string{message.Dest},
			MsgLength:          uint8(len(message.Content)),
			MsgContent:         message.Content,
		}
	}

	return &cmpp.Cmpp3SubmitReqPkt{
		PkTotal:            1,
		PkNumber:           1,
		RegisteredDelivery: 0,
		MsgLevel:           1,
		ServiceId:          cfg.ServiceId,
		FeeUserType:        0,
		FeeTerminalId:      "",
		FeeTerminalType:    0,
		MsgFmt:             0,
		MsgSrc:             cfg.User, // MsgSrc应该是企业代码，即登录用户名（6字节）
		FeeType:            "01",
		FeeCode:            "000000",
		ValidTime:          "",
		AtTime:             "",
		SrcId:              srcId,
		DestUsrTl:          1,
		DestTerminalId:     []string{message.Dest},
		DestTerminalType:   0,
		MsgLength:          uint8(len(message.Content)),
		MsgContent:         message.Content,
	}
}

package gateway

import (
	"testing"

	cmpp "github.com/bigwhite/gocmpp"
)

// TestParseCmppVersion 测试协议版本字符串解析
func TestParseCmppVersion(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  cmpp.Type
	}{
		{"2.0", "2.0", cmpp.V20},
		{"20", "20", cmpp.V20},
		{"v2.0", "v2.0", cmpp.V20},
		{"V2.0 uppercase", "V2.0", cmpp.V20},
		{"2.0 with spaces", "  2.0 ", cmpp.V20},
		{"3.0", "3.0", cmpp.V30},
		{"30", "30", cmpp.V30},
		{"empty defaults to 3.0", "", cmpp.V30},
		{"unknown defaults to 3.0", "4.0", cmpp.V30},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseCmppVersion(tt.input); got != tt.want {
				t.Errorf("ParseCmppVersion(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestIsV20 测试版本判断
func TestIsV20(t *testing.T) {
	if !IsV20(cmpp.V20) {
		t.Error("IsV20(V20) should be true")
	}
	if IsV20(cmpp.V30) {
		t.Error("IsV20(V30) should be false")
	}
}

// TestBuildSubmitReqPkt 测试按版本构造提交包
func TestBuildSubmitReqPkt(t *testing.T) {
	message := SmsMes{
		Src:     "123456",
		Dest:    "13800138000",
		Content: "test",
	}
	srcId := "1064899104221123456"

	t.Run("2.0 builds Cmpp2SubmitReqPkt", func(t *testing.T) {
		cfg := &Config{
			User:        "testuser",
			ServiceId:   "TEST",
			CmppVersion: "2.0",
		}
		p := buildSubmitReqPkt(cfg, message, srcId)
		pkt, ok := p.(*cmpp.Cmpp2SubmitReqPkt)
		if !ok {
			t.Fatalf("expected *cmpp.Cmpp2SubmitReqPkt, got %T", p)
		}
		if pkt.PkTotal != 1 || pkt.PkNumber != 1 {
			t.Errorf("unexpected PkTotal/PkNumber: %d/%d", pkt.PkTotal, pkt.PkNumber)
		}
		if pkt.ServiceId != "TEST" {
			t.Errorf("unexpected ServiceId: %q", pkt.ServiceId)
		}
		if pkt.MsgSrc != "testuser" {
			t.Errorf("unexpected MsgSrc: %q", pkt.MsgSrc)
		}
		if pkt.SrcId != srcId {
			t.Errorf("unexpected SrcId: %q", pkt.SrcId)
		}
		if len(pkt.DestTerminalId) != 1 || pkt.DestTerminalId[0] != "13800138000" {
			t.Errorf("unexpected DestTerminalId: %v", pkt.DestTerminalId)
		}
		if pkt.MsgContent != "test" || pkt.MsgLength != uint8(len("test")) {
			t.Errorf("unexpected MsgContent/MsgLength: %q/%d", pkt.MsgContent, pkt.MsgLength)
		}
		if pkt.FeeType != "01" || pkt.FeeCode != "000000" {
			t.Errorf("unexpected FeeType/FeeCode: %q/%q", pkt.FeeType, pkt.FeeCode)
		}
	})

	t.Run("3.0 builds Cmpp3SubmitReqPkt", func(t *testing.T) {
		cfg := &Config{
			User:        "testuser",
			ServiceId:   "TEST",
			CmppVersion: "3.0",
		}
		p := buildSubmitReqPkt(cfg, message, srcId)
		pkt, ok := p.(*cmpp.Cmpp3SubmitReqPkt)
		if !ok {
			t.Fatalf("expected *cmpp.Cmpp3SubmitReqPkt, got %T", p)
		}
		if pkt.ServiceId != "TEST" || pkt.MsgSrc != "testuser" || pkt.SrcId != srcId {
			t.Errorf("unexpected fields: %+v", pkt)
		}
		if len(pkt.DestTerminalId) != 1 || pkt.DestTerminalId[0] != "13800138000" {
			t.Errorf("unexpected DestTerminalId: %v", pkt.DestTerminalId)
		}
	})

	t.Run("empty version defaults to 3.0", func(t *testing.T) {
		cfg := &Config{
			User:      "testuser",
			ServiceId: "TEST",
		}
		p := buildSubmitReqPkt(cfg, message, srcId)
		if _, ok := p.(*cmpp.Cmpp3SubmitReqPkt); !ok {
			t.Fatalf("expected *cmpp.Cmpp3SubmitReqPkt for empty version, got %T", p)
		}
	})
}

// TestNewClientManagerVersion 测试 ClientManager 按配置选择协议版本
func TestNewClientManagerVersion(t *testing.T) {
	v20 := NewClientManager(&Config{CmppVersion: "2.0"})
	if v20.Version() != cmpp.V20 {
		t.Errorf("expected V20, got %v", v20.Version())
	}

	v30 := NewClientManager(&Config{CmppVersion: "3.0"})
	if v30.Version() != cmpp.V30 {
		t.Errorf("expected V30, got %v", v30.Version())
	}

	def := NewClientManager(&Config{})
	if def.Version() != cmpp.V30 {
		t.Errorf("expected default V30, got %v", def.Version())
	}
}

package type_event

import (
	"encoding/xml"
	"fmt"
	"testing"
)

func TestEventMenuPicSysPhoto(t *testing.T) {
	s := `<xml><ToUserName><![CDATA[gh_e136c6e50636]]></ToUserName>
<FromUserName><![CDATA[oMgHVjngRipVsoxg6TuX3vz6glDg]]></FromUserName>
<CreateTime>1408090651</CreateTime>
<MsgType><![CDATA[event]]></MsgType>
<Event><![CDATA[pic_sysphoto]]></Event>
<EventKey><![CDATA[6]]></EventKey>
<SendPicsInfo><Count>1</Count>
</SendPicsInfo>
</xml>`

	photo := EventMenuPicSysPhoto{}
	err := xml.Unmarshal([]byte(s), &photo)
	if err != nil {
		t.Fatalf("xml parser error=%+v\n", err)
	}
	fmt.Printf("photo=%+v\n", photo)
}

package httpapi

import "testing"

func TestFocusedTextUsesNonPasswordInputAndNormalizesCenter(t *testing.T) {
	document := `<hierarchy><node class="android.widget.LinearLayout"><node text="杭州 😀" class="android.widget.EditText" focused="true" password="false" bounds="[100,200][500,600]"/></node></hierarchy>`
	text, point, err := focusedText(document, 1000, 2000)
	if err != nil || text != "杭州 😀" || point.X != .3 || point.Y != .2 {
		t.Fatalf("text=%q point=%#v err=%v", text, point, err)
	}
}

func TestFocusedTextRejectsPassword(t *testing.T) {
	_, _, err := focusedText(`<hierarchy><node text="secret" class="android.widget.EditText" focused="true" password="true" bounds="[0,0][10,10]"/></hierarchy>`, 100, 100)
	if err == nil {
		t.Fatal("password input was accepted")
	}
}

func TestFocusedTextRejectsSensitiveMetadataWhenPasswordFlagIsFalse(t *testing.T) {
	for _, document := range []string{
		`<hierarchy><node text="plaintext-secret" class="android.widget.EditText" resource-id="com.example:id/password" focused="true" password="false" bounds="[0,0][10,10]"/></hierarchy>`,
		`<hierarchy><node text="123456" class="android.widget.EditText" content-desc="OTP verification code" focused="true" password="false" bounds="[0,0][10,10]"/></hierarchy>`,
		`<hierarchy><node text="••••" class="android.widget.EditText" focused="true" password="false" bounds="[0,0][10,10]"/></hierarchy>`,
	} {
		if _, _, err := focusedText(document, 100, 100); err == nil {
			t.Fatalf("sensitive input was accepted: %s", document)
		}
	}
}

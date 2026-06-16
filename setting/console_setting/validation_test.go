package console_setting

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestValidateAnnouncements_Allows3000ChineseCharacters(t *testing.T) {
	content := strings.Repeat("测", 3000)
	announcements := fmt.Sprintf(
		`[{"content":%q,"publishDate":%q,"type":"default"}]`,
		content,
		time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
	)

	if err := validateAnnouncements(announcements); err != nil {
		t.Fatalf("expected 3000 Chinese characters to pass, got error: %v", err)
	}
}

func TestValidateAnnouncements_Rejects3001ChineseCharacters(t *testing.T) {
	content := strings.Repeat("测", 3001)
	announcements := fmt.Sprintf(
		`[{"content":%q,"publishDate":%q,"type":"default"}]`,
		content,
		time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
	)

	err := validateAnnouncements(announcements)
	if err == nil {
		t.Fatal("expected 3001 Chinese characters to fail, got nil")
	}

	expected := "第1个公告的内容长度不能超过3000字符"
	if err.Error() != expected {
		t.Fatalf("expected error %q, got %q", expected, err.Error())
	}
}

func TestValidateAnnouncements_Allows3000EmojiCharacters(t *testing.T) {
	content := strings.Repeat("😀", 3000)
	announcements := fmt.Sprintf(
		`[{"content":%q,"publishDate":%q,"type":"default"}]`,
		content,
		time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
	)

	if err := validateAnnouncements(announcements); err != nil {
		t.Fatalf("expected 3000 emoji characters to pass, got error: %v", err)
	}
}

func TestValidateAnnouncements_Rejects3001EmojiCharacters(t *testing.T) {
	content := strings.Repeat("😀", 3001)
	announcements := fmt.Sprintf(
		`[{"content":%q,"publishDate":%q,"type":"default"}]`,
		content,
		time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
	)

	err := validateAnnouncements(announcements)
	if err == nil {
		t.Fatal("expected 3001 emoji characters to fail, got nil")
	}

	expected := "第1个公告的内容长度不能超过3000字符"
	if err.Error() != expected {
		t.Fatalf("expected error %q, got %q", expected, err.Error())
	}
}

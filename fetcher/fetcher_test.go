package fetcher

import (
	"reflect"
	"testing"
)

func TestChannelTabSuffixes_Video(t *testing.T) {
	got := ChannelTabSuffixes([]string{"video"})
	want := []string{"/videos"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ChannelTabSuffixes([video]): got %v, want %v", got, want)
	}
}

func TestChannelTabSuffixes_Live(t *testing.T) {
	got := ChannelTabSuffixes([]string{"live"})
	want := []string{"/streams"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ChannelTabSuffixes([live]): got %v, want %v", got, want)
	}
}

func TestChannelTabSuffixes_Short(t *testing.T) {
	got := ChannelTabSuffixes([]string{"short"})
	want := []string{"/shorts"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ChannelTabSuffixes([short]): got %v, want %v", got, want)
	}
}

func TestChannelTabSuffixes_VideoAndLive(t *testing.T) {
	got := ChannelTabSuffixes([]string{"video", "live"})
	want := []string{"/videos", "/streams"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ChannelTabSuffixes([video,live]): got %v, want %v", got, want)
	}
}

func TestChannelTabSuffixes_All(t *testing.T) {
	got := ChannelTabSuffixes([]string{"video", "live", "short"})
	want := []string{"/videos", "/streams", "/shorts"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ChannelTabSuffixes([video,live,short]): got %v, want %v", got, want)
	}
}

func TestChannelTabSuffixes_Empty(t *testing.T) {
	got := ChannelTabSuffixes([]string{})
	want := []string{"/videos", "/streams", "/shorts"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ChannelTabSuffixes([]): got %v, want %v", got, want)
	}
}

func TestChannelTabSuffixes_Nil(t *testing.T) {
	got := ChannelTabSuffixes(nil)
	want := []string{"/videos", "/streams", "/shorts"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ChannelTabSuffixes(nil): got %v, want %v", got, want)
	}
}

package planetary_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/RTradeLtd/Lens/xtractor/planetary"
	"github.com/RTradeLtd/config"
	"github.com/RTradeLtd/rtfs"
)

const (
	testHash      = "QmSi9TLyzTXmrLMXDvhztDoX3jghoG3vcRrnPkLvGgfpdW"
	testHashPdf   = "QmTbvUMmniE7wUP1ucbtC9s4ree7s8mSiQBt1c4odzKnY4"
	defaultConfig = "../../test/config.json"
)

func TestPlanetaryExtractor(t *testing.T) {
	if os.Getenv("TEST") != "integration" {
		t.Skip("skipping integration test", t.Name())
	}

	cfg, err := config.LoadConfig(defaultConfig)
	if err != nil {
		t.Fatal(err)
	}
	ipfsAPI := fmt.Sprintf("%s:%s", cfg.IPFS.APIConnection.Host, cfg.IPFS.APIConnection.Port)
	manager, err := rtfs.NewManager(ipfsAPI, "", 1*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	px := planetary.NewPlanetaryExtractor(manager)
	var out interface{}
	if err = px.ExtractObject(testHash, &out); err != nil {
		t.Fatal(err)
	}
	cidObj, err := planetary.DecodeStringToCID(testHash)
	if err != nil {
		t.Fatal(err)
	}
	expectedCodecUint := uint64(112) // hex 70, aka dag-protobuf codec
	if cidObj.Prefix().Codec != expectedCodecUint {
		t.Fatal("unexpected codec returned")
	}
	_, err = cidObj.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	_, err = px.ExtractContents(testHash)
	if err != nil {
		t.Fatal(err)
	}
}

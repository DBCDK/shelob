package handlers

import (
	"github.com/dbcdk/shelob/util"
	"testing"
	"time"
)

func createConfig(name string, lastUpdate time.Time, acceptableUpdateLag int, shutdownInProgress bool, hasBeenUpdated bool) *util.Config {
	return &util.Config{
		InstanceName:        name,
		LastUpdate:          lastUpdate,
		AcceptableUpdateLag: acceptableUpdateLag,
		State:               util.State{ShutdownInProgress: shutdownInProgress},
		HasBeenUpdated:      hasBeenUpdated,
	}
}

func TestCreateShelobStatus(t *testing.T) {
	if !CreateShelobStatus(createConfig("testing", time.Now().Add(-1*time.Hour), 0, false, true)).Ok {
		t.Error("Expected status=ok")
	}

	if !CreateShelobStatus(createConfig("testing", time.Now().Add(-1*time.Second), 5, false, true)).Ok {
		t.Error("Expected status=ok")
	}

	if CreateShelobStatus(createConfig("testing", time.Now().Add(-1*time.Hour), 10, false, true)).Ok {
		t.Error("Expected status=fail")
	}

	if CreateShelobStatus(createConfig("testing", time.Now().Add(-1*time.Hour), 0, true, true)).Ok {
		t.Error("Expected status=fail")
	}

	if CreateShelobStatus(createConfig("testing", time.Now().Add(-1*time.Second), 5, true, true)).Ok {
		t.Error("Expected status=fail")
	}

	if CreateShelobStatus(createConfig("testing", time.Now().Add(-1*time.Hour), 0, false, false)).Ok {
		t.Error("Expected status=fail")
	}

	if CreateShelobStatus(createConfig("testing", time.Now().Add(-1*time.Second), 5, false, false)).Ok {
		t.Error("Expected status=fail")
	}

	if CreateShelobStatus(createConfig("testing", time.Now().Add(-1*time.Hour), 0, true, false)).Ok {
		t.Error("Expected status=fail")
	}

	if CreateShelobStatus(createConfig("testing", time.Now().Add(-1*time.Second), 5, true, false)).Ok {
		t.Error("Expected status=fail")
	}
}

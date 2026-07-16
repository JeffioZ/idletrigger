package processcatalog

import "testing"

func TestIncludeSnapshotEntrySkipsSystemPseudoProcess(t *testing.T) {
	for _, test := range []struct {
		pid  uint32
		name string
		want bool
	}{
		{pid: 0, name: "[System Process]", want: false},
		{pid: 0, name: "System Idle Process", want: false},
		{pid: 4, name: "System", want: true},
		{pid: 42, name: "app.exe", want: true},
		{pid: 42, name: "", want: false},
	} {
		if got := includeSnapshotEntry(test.pid, test.name); got != test.want {
			t.Fatalf("includeSnapshotEntry(%d, %q) = %v, want %v", test.pid, test.name, got, test.want)
		}
	}
}

func TestGroupInstancesKeepsCountAndRepresentativeDescription(t *testing.T) {
	groups := GroupInstances([]Instance{
		{PID: 1, Executable: "app.exe", Path: `C:\A\app.exe`, Description: "App A"},
		{PID: 2, Executable: "APP.EXE", Path: `C:\A\app.exe`, Description: "App A"},
		{PID: 3, Executable: "app.exe", Path: `D:\B\app.exe`, Description: "App B"},
		{PID: 4, Executable: "app.exe"},
	})
	if len(groups) != 1 || groups[0].Count != 4 || groups[0].Description != "App A" {
		t.Fatalf("grouped instances = %+v", groups)
	}
}

func TestGroupInstancesDeduplicatesExecutableNames(t *testing.T) {
	groups := GroupInstances([]Instance{
		{PID: 1, Executable: "worker.exe"},
		{PID: 2, Executable: "WORKER.EXE"},
	})
	if len(groups) != 1 || groups[0].Count != 2 || groups[0].Executable != "worker.exe" {
		t.Fatalf("groups = %+v", groups)
	}
}

package main

import "testing"

func TestIsSSHRelatedUserProcess(t *testing.T) {
	cases := []struct {
		name    string
		comm    string
		cmdline string
		want    bool
	}{
		{name: "sshd comm", comm: "sshd", cmdline: "sshd: alice@pts/2", want: true},
		{name: "ssh-agent comm", comm: "ssh-agent", cmdline: "ssh-agent -s", want: true},
		{name: "sshd cmdline", comm: "bash", cmdline: "sshd: bob@pts/1", want: true},
		{name: "sftp server", comm: "internal-sftp", cmdline: "/usr/lib/openssh/sftp-server", want: true},
		{name: "normal process", comm: "python", cmdline: "python train.py", want: false},
	}
	for _, tc := range cases {
		if got := isSSHRelatedUserProcess(tc.comm, tc.cmdline); got != tc.want {
			t.Fatalf("%s: isSSHRelatedUserProcess(%q,%q)=%v want=%v", tc.name, tc.comm, tc.cmdline, got, tc.want)
		}
	}
}


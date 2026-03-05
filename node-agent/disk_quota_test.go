package main

import "testing"

func TestIsQuotaMountOpt(t *testing.T) {
	cases := []struct {
		name string
		opt  string
		want bool
	}{
		{name: "ext basic", opt: "usrquota", want: true},
		{name: "ext basic grp", opt: "grpquota", want: true},
		{name: "ext journal quota", opt: "usrjquota=aquota.user", want: true},
		{name: "ext journal grp", opt: "grpjquota=aquota.group", want: true},
		{name: "jqfmt", opt: "jqfmt=vfsv0", want: true},
		{name: "comma packed", opt: "rw,relatime,usrquota,grpquota", want: true},
		{name: "xfs pquota", opt: "pquota", want: true},
		{name: "xfs uquota", opt: "uquota", want: true},
		{name: "xfs no enforce", opt: "uqnoenforce", want: true},
		{name: "negative noquota", opt: "noquota", want: false},
		{name: "negative rw only", opt: "rw,relatime", want: false},
		{name: "negative empty", opt: "", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isQuotaMountOpt(tc.opt)
			if got != tc.want {
				t.Fatalf("isQuotaMountOpt(%q)=%v want=%v", tc.opt, got, tc.want)
			}
		})
	}
}


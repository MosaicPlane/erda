// Copyright (c) 2026 MosaicPlane Authors
// SPDX-License-Identifier: Apache-2.0

package oidc

import "testing"

func TestAdminSubjectsRequireExactIssuerAndSubject(t *testing.T) {
	p := &provider{Cfg: &Config{AdminSubjects: "https://id.example/realm|subject-1,\nhttps://other.example|subject-2"}}
	if !p.isAdminSubject("https://id.example/realm", "subject-1") {
		t.Fatal("expected exact identity to be admin")
	}
	if p.isAdminSubject("https://id.example/realm", "subject-2") {
		t.Fatal("subject from another issuer must not be admin")
	}
	if p.isAdminSubject("https://id.example/realm/", "subject-1") {
		t.Fatal("issuer comparison must be exact")
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "  ", "value"); got != "value" {
		t.Fatalf("got %q", got)
	}
}

package semtok

import (
	"testing"

	"alder/internal/syntax"
)

const declSrc = `module M

import Text as T

type Shape a = Circle Int | Square a
type Id = brand Text where Text.isEmpty
type Rec = { name: Text, tags: List Tag.Tag }

table Post @renamed "Article" {
  id: Int @id,
  author: User.UserId @ref User,
  org: Text @partition org,
}

page "/p/{slug}" { slug: Text, q: Text = "" as "Q" } requires user, org, role Admin | Editor, silo org

session { user: User.UserId, org: Maybe Org.OrgId via Membership }

silo org { Project, Task }

choose org = Orgs.Index

login = Auth.Login

deploy { prod: Ssh "host" { domain: "d", floor: -1 }, requires: ["x"] }

msg Save { title: Text } form NewPost

fake Mailer {
  table Sent { to: Text }
  send m = Db.insert Sent m
}

-- docs for f
f : Int -> Int
f n = n
`

func TestDecls(t *testing.T) {
	if _, ds := syntax.ParseFile("t.alder", []byte(declSrc)); len(ds) > 0 {
		t.Fatalf("the fixture does not parse: %v", ds)
	}
	toks := Module("t.alder", []byte(declSrc))
	checkInvariants(t, "declSrc", declSrc, toks)
	s := declSrc
	for _, c := range []struct {
		needle string
		off    int
		kind   Kind
		mods   int
	}{
		{"module M", 7, TNamespace, 0},
		{"import Text as T", 7, TNamespace, 0},
		{"import Text as T", 12, TKeyword, 0},
		{"import Text as T", 15, TNamespace, 0},
		{"type Shape a", 5, TType, Declaration},
		{"type Shape a", 11, TType, 0},
		{"Circle Int", 0, TEnumMember, Declaration},
		{"Circle Int", 7, TType, 0},
		{"| Square a", 2, TEnumMember, Declaration},
		{"| Square a", 9, TType, 0},
		{"brand Text where", 0, TKeyword, 0},
		{"brand Text where", 6, TType, 0},
		{"brand Text where", 11, TKeyword, 0},
		{"name: Text", 0, TProperty, 0},
		{"name: Text", 6, TType, 0},
		{"List Tag.Tag", 0, TType, 0},
		{"List Tag.Tag", 5, TNamespace, 0},
		{"List Tag.Tag", 9, TType, 0},
		{"table Post @renamed", 0, TKeyword, 0},
		{"table Post @renamed", 6, TEnumMember, 0},
		{"table Post @renamed", 11, TDecorator, 0},
		{"table Post @renamed", 12, TDecorator, 0},
		{"id: Int @id", 0, TProperty, 0},
		{"id: Int @id", 9, TDecorator, 0},
		{"@ref User", 5, TType, 0},
		{"@partition org", 11, TProperty, 0},
		{`as "Q"`, 0, TKeyword, 0},
		{"requires user, org", 0, TKeyword, 0},
		{"requires user, org", 9, TProperty, 0},
		{"requires user, org", 15, TProperty, 0},
		{"role Admin | Editor", 0, TKeyword, 0},
		{"role Admin | Editor", 5, TEnumMember, 0},
		{"role Admin | Editor", 11, TOperator, 0},
		{", silo org", 2, TKeyword, 0},
		{", silo org", 7, TProperty, 0},
		{"via Membership", 0, TKeyword, 0},
		{"via Membership", 4, TEnumMember, 0},
		{"silo org {", 0, TKeyword, 0},
		{"silo org {", 5, TProperty, 0},
		{"{ Project, Task }", 2, TType, 0},
		{"{ Project, Task }", 11, TType, 0},
		{"choose org = Orgs.Index", 0, TKeyword, 0},
		{"choose org = Orgs.Index", 7, TProperty, 0},
		{"choose org = Orgs.Index", 13, TNamespace, 0},
		{"choose org = Orgs.Index", 18, TNamespace, 0},
		{"login = Auth.Login", 0, TKeyword, 0},
		{"login = Auth.Login", 8, TNamespace, 0},
		{"login = Auth.Login", 13, TNamespace, 0},
		{"deploy { prod", 0, TKeyword, 0},
		{"deploy { prod", 9, TProperty, 0},
		{`Ssh "host"`, 0, TEnumMember, 0},
		{"domain:", 0, TProperty, 0},
		{"requires: [", 0, TProperty, 0},
		{"msg Save", 0, TKeyword, 0},
		{"msg Save", 4, TEnumMember, 0},
		{"title: Text }", 0, TProperty, 0},
		{"} form NewPost", 2, TKeyword, 0},
		{"} form NewPost", 7, TEnumMember, 0},
		{"fake Mailer", 0, TKeyword, 0},
		{"  table Sent", 2, TKeyword, 0},
		{"send m =", 0, TFunction, Declaration},
		{"send m =", 5, TParameter, Declaration},
		{"-- docs for f", 0, TComment, Documentation},
		{"f : Int", 0, TFunction, Declaration},
		{"f n = n", 0, TFunction, Declaration},
		{"f n = n", 2, TParameter, Declaration},
		{"f n = n", 6, TVariable, 0},
	} {
		want(t, s, toks, c.needle, c.off, c.kind, c.mods)
	}
	wantLen(t, s, toks, "floor: -1", 7, TNumber, 2) // `-1` is one number
	wantNone(t, s, toks, "floor: -1", 8)
	wantLen(t, s, toks, `"/p/{slug}"`, 0, TString, 11)
}

const routeSrc = `module Todos

route Create = POST "/todos" { } form NewTodo requires user, role Admin

Create.update : Create.Params -> Session -> Task Response
Create.update params session = Task.succeed (Response.refresh)
`

func TestRouteDecls(t *testing.T) {
	if _, ds := syntax.ParseFile("t.alder", []byte(routeSrc)); len(ds) > 0 {
		t.Fatalf("the fixture does not parse: %v", ds)
	}
	toks := Module("t.alder", []byte(routeSrc))
	checkInvariants(t, "routeSrc", routeSrc, toks)
	for _, c := range []struct {
		needle string
		off    int
		kind   Kind
		mods   int
	}{
		{"route Create", 0, TKeyword, 0},
		{"Create =", 0, TEnumMember, 0},
		{"POST", 0, TEnumMember, 0},
		{"form NewTodo", 0, TKeyword, 0},
		{"requires user", 9, TProperty, 0},
		{"role Admin", 0, TKeyword, 0},
		{"Create.update :", 0, TNamespace, 0},
		{"Create.update :", 7, TFunction, Declaration},
		{"Create.update params", 0, TNamespace, 0},
		{"Create.update params", 7, TFunction, Declaration},
	} {
		want(t, routeSrc, toks, c.needle, c.off, c.kind, c.mods)
	}
}

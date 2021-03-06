// Package rte provides simple performant routing.
// - Define individual routes with `rte.Func`,
// - Combine them into a table with `rte.Must` or `rte.New`
// - Access wildcard matched variables with `rte.PathVars`
package rte

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// Func routes requests matching the method and path to a handler
func Func(method, path string, f func(http.ResponseWriter, *http.Request)) Route {
	return Route{Method: method, Path: path, Handler: http.HandlerFunc(f)}
}

// Route is data for routing to a handler
type Route struct {
	Method, Path string
	Handler      http.Handler
}

// Must builds routes into a Table and panics if there's an error
func Must(routes ...Route) *Table {
	t, e := New(routes...)
	if e != nil {
		panic(e.Error())
	}
	return t
}

// New builds routes into a Table or returns an error
func New(routes ...Route) (*Table, error) {
	t := new(Table)

	t.m = make(map[string]*node)

	for i, r := range routes {
		if r.Method == "" {
			return nil, fmt.Errorf("route %v: Method cannot be empty", i)
		}

		if r.Handler == nil {
			return nil, fmt.Errorf("route %v: handle cannot be nil", i)
		}

		if r.Path == "" {
			return nil, fmt.Errorf("route %v: Path cannot be empty", i)
		}

		if r.Path[0] != '/' {
			return nil, fmt.Errorf("route %v: must start with / -- got %#v", i, r.Path)
		}

		if t.m[r.Method] == nil {
			t.m[r.Method] = &node{children: make(map[string]*node)}
		}
		n := t.m[r.Method]

		for _, seg := range strings.SplitAfter(r.Path, "/")[1:] {
			if n.children[seg] == nil {
				n.children[seg] = &node{children: make(map[string]*node)}
			}
			n = n.children[seg]
		}

		if n.h != nil {
			return nil, fmt.Errorf("route %v: already has a handler for %v %#v", i, r.Method, r.Path)
		}

		n.h = r.Handler
	}

	t.Default = http.NotFoundHandler()

	return t, nil
}

// Table manages the routing table and a default handler
type Table struct {
	m       map[string]*node
	Default http.Handler
}

func (t *Table) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if t.m[r.Method] == nil {
		t.Default.ServeHTTP(w, r)
		return
	}

	n := t.m[r.Method]
	var params []string
	for _, seg := range strings.SplitAfter(r.URL.Path, "/")[1:] {
		var m string
		if m, n = n.match(seg); n == nil {
			t.Default.ServeHTTP(w, r)
			return
		} else if m != "" {
			params = append(params, m)
		}
	}

	if n.h == nil {
		t.Default.ServeHTTP(w, r)
		return
	}

	*r = *r.WithContext(context.WithValue(r.Context(), pathVarKey, params))
	n.h.ServeHTTP(w, r)
}

// PathVars returns the values for any matched wildcards in the order they were found
func PathVars(r *http.Request) []string {
	if val := r.Context().Value(pathVarKey); val != nil {
		return val.([]string)
	}
	return nil
}

type key int

const (
	pathVarKey key = 0
)

type node struct {
	children map[string]*node
	h        http.Handler
}

func (n *node) match(seg string) (string, *node) {
	if c := n.children[seg]; c != nil {
		return "", c
	} else if l := len(seg) - 1; l >= 0 && seg[l] == '/' {
		return seg[:l], n.children["*/"]
	} else {
		return seg, n.children["*"]
	}
}

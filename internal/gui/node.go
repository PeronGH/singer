package gui

import (
	"net/url"
	"strings"
)

type nodeEntry struct {
	Index int
	URI   string
	Tag   string
	Desc  string
}

func extractTag(uri string) string {
	if idx := strings.LastIndex(uri, "#"); idx >= 0 && idx < len(uri)-1 {
		return uri[idx+1:]
	}
	return uri
}

func extractDesc(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return uri
	}
	host := u.Hostname()
	if port := u.Port(); port != "" {
		host += ":" + port
	}
	if u.Scheme == "" {
		return host
	}
	if host == "" {
		return u.Scheme
	}
	return u.Scheme + " · " + host
}

func filterNodes(nodes []string, query string) []nodeEntry {
	q := strings.ToLower(strings.TrimSpace(query))
	out := make([]nodeEntry, 0, len(nodes))
	for i, uri := range nodes {
		tag := extractTag(uri)
		desc := extractDesc(uri)
		if q != "" &&
			!strings.Contains(strings.ToLower(tag), q) &&
			!strings.Contains(strings.ToLower(desc), q) &&
			!strings.Contains(strings.ToLower(uri), q) {
			continue
		}
		out = append(out, nodeEntry{
			Index: i,
			URI:   uri,
			Tag:   tag,
			Desc:  desc,
		})
	}
	return out
}

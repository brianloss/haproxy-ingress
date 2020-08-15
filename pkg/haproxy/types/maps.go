/*
Copyright 2020 The HAProxy Ingress Controller Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package types

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// CreateMaps ...
func CreateMaps() *HostsMaps {
	return &HostsMaps{}
}

// AddMap ...
func (hm *HostsMaps) AddMap(basename string) *HostsMap {
	hmap := &HostsMap{
		basename:  basename,
		filenames: map[MatchType]string{},
		values:    map[MatchType][]*HostsMapEntry{},
	}
	hm.Items = append(hm.Items, hmap)
	return hmap
}

// AddHostnameMapping ...
func (hm *HostsMap) AddHostnameMapping(hostname, target string) {
	hostname, hasWildcard := convertWildcardToRegex(hostname, true)
	if hasWildcard {
		hm.addTarget(hostname, "", target, MatchRegex)
	} else {
		hm.addTarget(hostname, "", target, MatchExact)
	}
}

// AddHostnamePathMapping ...
func (hm *HostsMap) AddHostnamePathMapping(hostname string, hostPath *HostPath, target string) {
	hostname, hasWildcard := convertWildcardToRegex(hostname, false)
	path := hostPath.Path
	match := hostPath.Match
	// TODO paths of a wildcard hostname will always have less precedence
	// despite the match type because the whole hostname+path will fill a
	// MatchRegex map, which has the lesser precedence in the template.
	if hasWildcard {
		path = convertPathToRegex(hostPath)
		match = MatchRegex
	} else if hostPath.Match == MatchRegex {
		hostname = "^" + regexp.QuoteMeta(hostname)
		path = hostPath.Path + "$"
	}
	hm.addTarget(hostname, path, target, match)
}

// AddAliasPathMapping ...
func (hm *HostsMap) AddAliasPathMapping(alias HostAliasConfig, path *HostPath, target string) {
	if alias.AliasName != "" {
		hm.AddHostnamePathMapping(alias.AliasName, path, target)
	}
	if alias.AliasRegex != "" {
		pathstr := convertPathToRegex(path)
		hm.addTarget("^"+alias.AliasRegex, pathstr, target, MatchRegex)
	}
}

func convertWildcardToRegex(hostname string, matchEol bool) (h string, hasWildcard bool) {
	if !strings.HasPrefix(hostname, "*.") {
		return hostname, false
	}
	hostregex := "^[^.]+" + regexp.QuoteMeta(hostname[1:])
	if matchEol {
		return hostregex + "$", true
	}
	return hostregex, true
}

func convertPathToRegex(hostPath *HostPath) string {
	switch hostPath.Match {
	case MatchBegin:
		return regexp.QuoteMeta(hostPath.Path)
	case MatchExact:
		return regexp.QuoteMeta(hostPath.Path) + "$"
	case MatchPrefix:
		path := regexp.QuoteMeta(hostPath.Path)
		if strings.HasSuffix(path, "/") {
			return path
		}
		return path + "(/.*)?$"
	case MatchRegex:
		return hostPath.Path + "$"
	}
	panic("unsupported match type")
}

func (hm *HostsMap) addTarget(hostname, path, target string, match MatchType) {
	hostname = strings.ToLower(hostname)
	if match == MatchBegin {
		// this is the only match that uses case insensitive path
		path = strings.ToLower(path)
	}
	entry := &HostsMapEntry{
		hostname: hostname,
		path:     path,
		Key:      hostname + path,
		Value:    target,
	}
	values := hm.values[match]
	values = append(values, entry)
	if match == MatchRegex {
		// Keep regexes in order from most to least specific, based on rule length
		sort.Slice(values, func(i, j int) bool {
			k1 := values[i].Key
			k2 := values[j].Key
			if len(k1) != len(k2) {
				return len(k1) > len(k2)
			}
			return k1 < k2
		})
	} else {
		// Ascending order of hostnames and reverse order of paths within the same hostname
		sort.Slice(values, func(i, j int) bool {
			v1 := values[i]
			v2 := values[j]
			if v1.hostname == v2.hostname {
				return v1.path > v2.path
			}
			return v1.Key < v2.Key
		})
	}
	hm.values[match] = values
}

// Matches ...
func (hm *HostsMap) Matches() []MatchType {
	var matches []MatchType
	for match := range hm.values {
		matches = append(matches, match)
	}
	return matches
}

// Values ...
func (hm *HostsMap) Values(match MatchType) []*HostsMapEntry {
	return hm.values[match]
}

// AppendItem ...
func (hm *HostsMap) AppendItem(item string) {
	values := hm.values[MatchEmpty]
	values = append(values, &HostsMapEntry{
		Key: item,
	})
	hm.values[MatchEmpty] = values
}

// HasHost ...
func (hm *HostsMap) HasHost() bool {
	for _, values := range hm.values {
		if len(values) > 0 {
			return true
		}
	}
	return false
}

// Has ...
func (hm *HostsMap) Has(match MatchType) bool {
	return len(hm.values[match]) > 0
}

// HasBegin ...
func (hm *HostsMap) HasBegin() bool {
	return hm.Has(MatchBegin)
}

// HasExact ...
func (hm *HostsMap) HasExact() bool {
	return hm.Has(MatchExact)
}

// HasPrefix ...
func (hm *HostsMap) HasPrefix() bool {
	return hm.Has(MatchPrefix)
}

// HasRegex ...
func (hm *HostsMap) HasRegex() bool {
	return hm.Has(MatchRegex)
}

// Filename ...
func (hm *HostsMap) Filename(match MatchType) (string, error) {
	if !hm.Has(match) {
		return "", fmt.Errorf("file content is empty")
	}
	filename, found := hm.filenames[match]
	if !found {
		if match == MatchEmpty {
			filename = hm.basename
		} else {
			filename = strings.Replace(hm.basename, ".", "__"+string(match)+".", 1)
		}
		hm.filenames[match] = filename
	}
	return filename, nil
}

// FilenameBegin ...
func (hm *HostsMap) FilenameBegin() (string, error) {
	return hm.Filename(MatchBegin)
}

// FilenameExact ...
func (hm *HostsMap) FilenameExact() (string, error) {
	return hm.Filename(MatchExact)
}

// FilenamePrefix ...
func (hm *HostsMap) FilenamePrefix() (string, error) {
	return hm.Filename(MatchPrefix)
}

// FilenameRegex ...
func (hm *HostsMap) FilenameRegex() (string, error) {
	return hm.Filename(MatchRegex)
}

// FilenameEmpty ...
func (hm *HostsMap) FilenameEmpty() (string, error) {
	return hm.Filename(MatchEmpty)
}

func (he *HostsMapEntry) String() string {
	return fmt.Sprintf("%+v", *he)
}

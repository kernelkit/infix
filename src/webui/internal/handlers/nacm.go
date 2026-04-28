// SPDX-License-Identifier: MIT

package handlers

import (
	"html/template"
	"log"
	"net/http"
	"strings"

	"github.com/kernelkit/webui/internal/restconf"
)

// ─── RESTCONF JSON types ──────────────────────────────────────────────────────

type nacmWrapper struct {
	NACM nacmJSON `json:"ietf-netconf-acm:nacm"`
}

type nacmJSON struct {
	EnableNACM          bool          `json:"enable-nacm"`
	ReadDefault         string        `json:"read-default"`
	WriteDefault        string        `json:"write-default"`
	ExecDefault         string        `json:"exec-default"`
	DeniedOperations    uint32        `json:"denied-operations"`
	DeniedDataWrites    uint32        `json:"denied-data-writes"`
	DeniedNotifications uint32        `json:"denied-notifications"`
	Groups              nacmGroupsJSON `json:"groups"`
	RuleList            []nacmRuleListJSON `json:"rule-list"`
}

type nacmGroupsJSON struct {
	Group []nacmGroupJSON `json:"group"`
}

type nacmGroupJSON struct {
	Name     string   `json:"name"`
	UserName []string `json:"user-name"`
}

type nacmRuleListJSON struct {
	Name  string           `json:"name"`
	Group []string         `json:"group"`
	Rule  []nacmRuleJSON   `json:"rule"`
}

type nacmRuleJSON struct {
	Name             string `json:"name"`
	ModuleName       string `json:"module-name"`
	Path             string `json:"path"`
	AccessOperations string `json:"access-operations"`
	Action           string `json:"action"`
}

type nacmAuthWrapper struct {
	System struct {
		Authentication struct {
			User []nacmUserJSON `json:"user"`
		} `json:"authentication"`
	} `json:"ietf-system:system"`
}

type nacmUserJSON struct {
	Name          string        `json:"name"`
	Password      string        `json:"password"`
	Shell         string        `json:"infix-system:shell"`
	AuthorizedKey []interface{} `json:"authorized-key"`
}

// ─── Template data ────────────────────────────────────────────────────────────

type nacmPageData struct {
	PageData
	Error string

	// Summary card
	Enabled             string
	ReadDefault         string
	WriteDefault        string
	ExecDefault         string
	DeniedOperations    uint32
	DeniedDataWrites    uint32
	DeniedNotifications uint32

	// Permission matrix
	Matrix []nacmGroupPerm

	// Users and Groups tables
	Users  []nacmUserEntry
	Groups []nacmGroupEntry
}

type nacmGroupPerm struct {
	Name        string
	Read        nacmCell
	Write       nacmCell
	Exec        nacmCell
	Restrictions []string
}

type nacmCell struct {
	Class  string // "nacm-full" | "nacm-restricted" | "nacm-denied"
	Symbol string // "✓" | "⚠" | "✗"
}

type nacmUserEntry struct {
	Name  string
	Shell string
	Login string
}

type nacmGroupEntry struct {
	Name    string
	Members string
}

// ─── Handler ─────────────────────────────────────────────────────────────────

// NACMHandler serves the NACM page.
type NACMHandler struct {
	Template *template.Template
	RC       *restconf.Client
}

// Overview renders the NACM page (GET /nacm).
func (h *NACMHandler) Overview(w http.ResponseWriter, r *http.Request) {
	data := nacmPageData{
		PageData: newPageData(r, "nacm", "NACM"),
	}

	var nacmRaw nacmWrapper
	nacmErr := h.RC.Get(r.Context(), "/data/ietf-netconf-acm:nacm", &nacmRaw)

	var authRaw nacmAuthWrapper
	authErr := h.RC.Get(r.Context(), "/data/ietf-system:system/authentication", &authRaw)

	if nacmErr != nil {
		log.Printf("restconf nacm: %v", nacmErr)
		data.Error = "Could not fetch NACM data"
	} else {
		n := nacmRaw.NACM
		if n.ReadDefault == "" {
			n.ReadDefault = "permit"
		}
		if n.WriteDefault == "" {
			n.WriteDefault = "deny"
		}
		if n.ExecDefault == "" {
			n.ExecDefault = "permit"
		}

		if n.EnableNACM {
			data.Enabled = "yes"
		} else {
			data.Enabled = "no"
		}
		data.ReadDefault = n.ReadDefault
		data.WriteDefault = n.WriteDefault
		data.ExecDefault = n.ExecDefault
		data.DeniedOperations = n.DeniedOperations
		data.DeniedDataWrites = n.DeniedDataWrites
		data.DeniedNotifications = n.DeniedNotifications

		data.Matrix = analyzeNACMPermissions(n)

		for _, g := range n.Groups.Group {
			data.Groups = append(data.Groups, nacmGroupEntry{
				Name:    g.Name,
				Members: strings.Join(g.UserName, " "),
			})
		}
	}

	if authErr != nil {
		log.Printf("restconf nacm auth: %v", authErr)
	} else {
		for _, u := range authRaw.System.Authentication.User {
			data.Users = append(data.Users, buildNACMUserEntry(u))
		}
	}

	tmplName := "nacm.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.Template.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// ─── Permission matrix logic ──────────────────────────────────────────────────
// Mirrors cli_pretty._analyze_group_permissions exactly.

func analyzeNACMPermissions(n nacmJSON) []nacmGroupPerm {
	readDefault := n.ReadDefault == "permit"
	writeDefault := n.WriteDefault == "permit"
	execDefault := n.ExecDefault == "permit"

	// Collect deny rules that apply to "*" (all groups) — these become restrictions.
	var globalDenials []nacmRuleJSON
	for _, rl := range n.RuleList {
		for _, g := range rl.Group {
			if g == "*" {
				for _, rule := range rl.Rule {
					if rule.Action == "deny" {
						globalDenials = append(globalDenials, rule)
					}
				}
				break
			}
		}
	}

	var results []nacmGroupPerm

	for _, group := range n.Groups.Group {
		canRead := readDefault
		canWrite := writeDefault
		canExec := execDefault
		hasPermitAll := false
		var restrictions []string

		// Process rule-lists in order; only those that name this group specifically.
		for _, rl := range n.RuleList {
			applies := false
			for _, g := range rl.Group {
				if g == group.Name {
					applies = true
					break
				}
			}
			if !applies {
				continue
			}

			for _, rule := range rl.Rule {
				action := rule.Action
				accessOps := strings.ToLower(rule.AccessOperations)
				moduleName := rule.ModuleName

				// permit-all: module-name="*" AND access-operations="*"
				if action == "permit" && moduleName == "*" && accessOps == "*" {
					hasPermitAll = true
					break
				}

				// Explicit deny for write/exec operations
				if action == "deny" && moduleName == "*" {
					if strings.Contains(accessOps, "create") &&
						strings.Contains(accessOps, "update") &&
						strings.Contains(accessOps, "delete") {
						canWrite = false
					}
					if strings.Contains(accessOps, "exec") {
						canExec = false
					}
				}
			}

			if hasPermitAll {
				break
			}
		}

		// permit-all overrides everything, including unfavourable global defaults.
		if hasPermitAll {
			canRead = true
			canWrite = true
			canExec = true
		}

		// Global denials create restrictions for groups that don't have permit-all.
		if !hasPermitAll {
			seen := map[string]bool{}
			for _, rule := range globalDenials {
				var restriction string
				if rule.Path != "" {
					parts := strings.Split(strings.TrimRight(rule.Path, "/"), "/")
					restriction = parts[len(parts)-1]
				} else if rule.ModuleName != "" {
					restriction = strings.TrimPrefix(rule.ModuleName, "ietf-")
				}
				if restriction != "" && !seen[restriction] {
					seen[restriction] = true
					restrictions = append(restrictions, restriction)
				}
			}
		}

		results = append(results, nacmGroupPerm{
			Name:         group.Name,
			Read:         makeNACMCell(canRead, len(restrictions) > 0),
			Write:        makeNACMCell(canWrite, len(restrictions) > 0),
			Exec:         makeNACMCell(canExec, len(restrictions) > 0),
			Restrictions: restrictions,
		})
	}

	return results
}

func makeNACMCell(hasAccess, hasRestrictions bool) nacmCell {
	if !hasAccess {
		return nacmCell{Class: "nacm-denied", Symbol: "✗"}
	}
	if hasRestrictions {
		return nacmCell{Class: "nacm-restricted", Symbol: "⚠"}
	}
	return nacmCell{Class: "nacm-full", Symbol: "✓"}
}

// ─── User entry helper ────────────────────────────────────────────────────────

func buildNACMUserEntry(u nacmUserJSON) nacmUserEntry {
	shell := u.Shell
	if idx := strings.LastIndex(shell, ":"); idx >= 0 {
		shell = shell[idx+1:]
	}
	if shell == "" || shell == "false" {
		shell = "-"
	}

	var login string
	hasPassword := u.Password != ""
	hasKeys := len(u.AuthorizedKey) > 0
	switch {
	case hasPassword && hasKeys:
		login = "password+key"
	case hasPassword:
		login = "password"
	case hasKeys:
		login = "key"
	default:
		login = "-"
	}

	return nacmUserEntry{
		Name:  u.Name,
		Shell: shell,
		Login: login,
	}
}

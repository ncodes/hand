package guardrails

import (
	"regexp"
	"strings"
)

type CommandPolicy struct {
	Allow []string
	Ask   []string
	Deny  []string
}

type CommandDecision string

const (
	CommandAllowed          CommandDecision = "allowed"
	CommandApprovalRequired CommandDecision = "approval_required"
	CommandDenied           CommandDecision = "denied"
)

type CommandEvaluation struct {
	Decision CommandDecision
	Rule     string
	Reason   string
}

type dangerousCommandPattern struct {
	reason  string
	pattern *regexp.Regexp
	match   func(string) bool
}

var builtInApprovalPatterns = []dangerousCommandPattern{
	{
		reason:  "dangerous destructive command",
		pattern: regexp.MustCompile(`^rm (?:-[^ ]*[rR][^ ]*|--recursive)(?: .*?)?/$`),
	},
	{
		reason:  "delete in root path",
		pattern: regexp.MustCompile(`^rm(?: .+)? /$`),
	},
	{
		reason:  "world-writable permissions command",
		pattern: regexp.MustCompile(`^chmod (?:777|0777|a\+rwx)( |$)`),
	},
	{
		reason:  "recursive world-writable permissions command",
		pattern: regexp.MustCompile(`^chmod (?:-[^ ]*R|--recursive\b).*(?:\b777\b|\b0777\b|a\+rwx\b)`),
	},
	{
		reason:  "recursive chown to root command",
		pattern: regexp.MustCompile(`^chown (?:-[^ ]*R|--recursive\b).* root\b`),
	},
	{
		reason:  "privileged shutdown command",
		pattern: regexp.MustCompile(`^sudo (reboot|shutdown|poweroff)( |$)`),
	},
	{
		reason:  "disk formatting command",
		pattern: regexp.MustCompile(`^mkfs(\.[^ ]+)?( |$)`),
	},
	{
		reason:  "shutdown command",
		pattern: regexp.MustCompile(`^(shutdown|reboot|poweroff|halt)( |$)`),
	},
	{
		reason:  "disk copy command",
		pattern: regexp.MustCompile(`^dd( |$).*?\bif=`),
	},
	{
		reason:  "sql drop command",
		pattern: regexp.MustCompile(`\bDROP (TABLE|DATABASE)\b`),
	},
	{
		reason: "sql delete without where command",
		match: func(joined string) bool {
			return deleteFromPattern.MatchString(joined) && !wherePattern.MatchString(joined)
		},
	},
	{
		reason:  "sql truncate command",
		pattern: regexp.MustCompile(`\bTRUNCATE TABLE\b`),
	},
	{
		reason:  "system config overwrite command",
		pattern: regexp.MustCompile(`> /etc/`),
	},
	{
		reason:  "block device overwrite command",
		pattern: regexp.MustCompile(`> /dev/sd`),
	},
	{
		reason:  "system service disable command",
		pattern: regexp.MustCompile(`^(?:sudo )?systemctl(?: --[^ ]+)* (stop|disable|mask)( |$)`),
	},
	{
		reason:  "kill all processes command",
		pattern: regexp.MustCompile(`^kill (?:-9|-KILL|-s KILL) -1$`),
	},
	{
		reason:  "force kill processes command",
		pattern: regexp.MustCompile(`^pkill (?:-9|-KILL|--signal(?:=| )9|--signal(?:=| )KILL)\b`),
	},
	{
		reason: "fork bomb command",
		match:  isForkBombCommand,
	},
	{
		reason:  "download and execute chain",
		pattern: regexp.MustCompile(`\b(curl|wget)\b.*\|.*\b(sh|bash)\b`),
	},
	{
		reason:  "execute remote script via process substitution",
		pattern: regexp.MustCompile(`\b(bash|sh|zsh|ksh)\b\s+<\s*\(\s*(curl|wget)\b`),
	},
	{
		reason:  "overwrite system file via tee",
		pattern: regexp.MustCompile(`\btee\b.*(/etc/|/dev/sd|\.ssh/|\.hermes/\.env)`),
	},
	{
		reason:  "xargs with rm command",
		pattern: regexp.MustCompile(`\bxargs\b.*\brm\b`),
	},
	{
		reason:  "shell execution via flag",
		pattern: regexp.MustCompile(`^(bash|sh|zsh|ksh) -[^ ]*c( |$)`),
	},
	{
		reason:  "script execution via flag",
		pattern: regexp.MustCompile(`^(python[23]?|perl|ruby|node) -(c|e)( |$)`),
	},
	{
		reason:  "find destructive action command",
		pattern: regexp.MustCompile(`^find\b.*(\-exec rm\b|\-delete\b)`),
	},
	{
		reason:  "credential exfiltration command",
		pattern: regexp.MustCompile(`\bcat (\.env|\.netrc)\b`),
	},
}

var (
	deleteFromPattern = regexp.MustCompile(`\bDELETE FROM\b`)
	wherePattern      = regexp.MustCompile(`\bWHERE\b`)
	forkBombPatterns  = []*regexp.Regexp{
		regexp.MustCompile(`:\(\)\s*\{\s*:\|:\s*&\s*\};:`),
		regexp.MustCompile(`\b\w+\(\)\s*\{\s*\w+\|\w+\s*&\s*\};\s*\w+`),
		regexp.MustCompile(`%0\|%0`),
		regexp.MustCompile(`\b(start|call)\s+.*%0\b`),
		regexp.MustCompile(`\bfor /l\b.*\bstart\b.*\bcmd\b`),
		regexp.MustCompile(`\bpython(?:\d+(?:\.\d+)?)?\b.*\b(subprocess\.(Popen|run)|os\.(fork|spawn|system)|multiprocessing\.Process)\b.*(__file__|sys\.argv\[0\]|python)`),
		regexp.MustCompile(`\b(node|ruby|perl|php)\b.*\b(spawn|fork|exec|system)\b.*(process\.argv|__FILE__|\$0|argv)`),
	}
)

func (p CommandPolicy) Normalize() CommandPolicy {
	p.Allow = normalizeCommandRules(p.Allow)
	p.Ask = normalizeCommandRules(p.Ask)
	p.Deny = normalizeCommandRules(p.Deny)
	return p
}

func EvaluateCommand(policy CommandPolicy, command string, args []string) CommandEvaluation {
	policy = policy.Normalize()
	tokens := commandTokens(command, args)
	if len(tokens) == 0 {
		return CommandEvaluation{Decision: CommandDenied, Reason: "empty command"}
	}

	if rule := matchCommandRule(policy.Deny, tokens); rule != "" {
		return CommandEvaluation{Decision: CommandDenied, Rule: rule, Reason: "matched deny rule"}
	}

	if reason := builtInApprovalRequired(tokens); reason != "" {
		return CommandEvaluation{Decision: CommandApprovalRequired, Reason: reason}
	}

	if rule := matchCommandRule(policy.Ask, tokens); rule != "" {
		return CommandEvaluation{Decision: CommandApprovalRequired, Rule: rule, Reason: "matched approval rule"}
	}

	if rule := matchCommandRule(policy.Allow, tokens); rule != "" {
		return CommandEvaluation{Decision: CommandAllowed, Rule: rule}
	}

	return CommandEvaluation{Decision: CommandAllowed}
}

func normalizeCommandRules(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))

	for _, value := range values {
		value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
		if value == "" {
			continue
		}

		if _, ok := seen[value]; ok {
			continue
		}

		seen[value] = struct{}{}
		out = append(out, value)
	}

	return out
}

func commandTokens(command string, args []string) []string {
	if len(args) > 0 {
		tokens := []string{strings.TrimSpace(command)}
		for _, arg := range args {
			trimmed := strings.TrimSpace(arg)
			if trimmed == "" {
				continue
			}
			tokens = append(tokens, trimmed)
		}
		return normalizeTokens(tokens)
	}

	return normalizeTokens(strings.Fields(command))
}

func normalizeTokens(tokens []string) []string {
	out := make([]string, 0, len(tokens))

	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		out = append(out, token)
	}

	return out
}

func matchCommandRule(rules []string, tokens []string) string {
	for _, rule := range rules {
		ruleTokens := strings.Fields(rule)
		if len(ruleTokens) == 0 || len(ruleTokens) > len(tokens) {
			continue
		}

		matched := true
		for i := range ruleTokens {
			if ruleTokens[i] != tokens[i] {
				matched = false
				break
			}
		}

		if matched {
			return rule
		}
	}

	return ""
}

func builtInApprovalRequired(tokens []string) string {
	joined := strings.Join(tokens, " ")

	for _, candidate := range builtInApprovalPatterns {
		if candidate.pattern != nil && candidate.pattern.MatchString(joined) {
			return candidate.reason
		}

		if candidate.match != nil && candidate.match(joined) {
			return candidate.reason
		}
	}

	return ""
}

func isForkBombCommand(joined string) bool {
	for _, pattern := range forkBombPatterns {
		if pattern.MatchString(joined) {
			return true
		}
	}

	return false
}

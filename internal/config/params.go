package config

import (
	"fmt"
	"regexp"
	"strings"
)

var routePlaceholderPattern = regexp.MustCompile(`\{([A-Za-z][A-Za-z0-9_-]*)\}`)
var targetPlaceholderPattern = regexp.MustCompile(`\{([A-Za-z][A-Za-z0-9_.-]*)\}`)

type ParamRule struct {
	Allow []string `yaml:"allow"`
}

func RouteParamNames(pattern string) ([]string, error) {
	matches := routePlaceholderPattern.FindAllStringSubmatch(pattern, -1)
	names := make([]string, 0, len(matches))
	seen := map[string]bool{}
	for _, match := range matches {
		name := match[1]
		if seen[name] {
			return nil, fmt.Errorf("duplicate route param %q", name)
		}
		seen[name] = true
		names = append(names, name)
	}
	if strings.Contains(pattern, "{") || strings.Contains(pattern, "}") {
		replaced := routePlaceholderPattern.ReplaceAllString(pattern, "")
		if strings.Contains(replaced, "{") || strings.Contains(replaced, "}") {
			return nil, fmt.Errorf("invalid route placeholder syntax")
		}
	}
	return names, nil
}

func TargetPlaceholderNames(target string) ([]string, error) {
	matches := targetPlaceholderPattern.FindAllStringSubmatch(target, -1)
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		names = append(names, match[1])
	}
	if strings.Contains(target, "{") || strings.Contains(target, "}") {
		replaced := targetPlaceholderPattern.ReplaceAllString(target, "")
		if strings.Contains(replaced, "{") || strings.Contains(replaced, "}") {
			return nil, fmt.Errorf("invalid target placeholder syntax")
		}
	}
	return names, nil
}

func ValidateRouteParams(route RouteDef) error {
	names, err := RouteParamNames(route.From)
	if err != nil {
		return err
	}
	if len(names) > 0 && strings.Contains(route.From, "*") {
		return fmt.Errorf("cannot mix named params and wildcard in from")
	}

	defined := map[string]bool{}
	for _, name := range names {
		defined[name] = true
	}
	for name, rule := range route.Params {
		if !defined[name] {
			return fmt.Errorf("params.%s is not declared in from", name)
		}
		if len(rule.Allow) == 0 {
			continue
		}
		seen := map[string]bool{}
		for _, value := range rule.Allow {
			if value == "" {
				return fmt.Errorf("params.%s.allow contains empty value", name)
			}
			if strings.Contains(value, "/") {
				return fmt.Errorf("params.%s.allow value %q must not contain /", name, value)
			}
			if seen[value] {
				return fmt.Errorf("params.%s.allow contains duplicate %q", name, value)
			}
			seen[value] = true
		}
	}

	targetNames, err := TargetPlaceholderNames(route.To)
	if err != nil {
		return err
	}
	for _, name := range targetNames {
		if defined[name] || name == "git.username" {
			continue
		}
		return fmt.Errorf("target placeholder %q is not available", name)
	}
	return nil
}

func InterpolateTarget(target string, params map[string]string, global *GlobalConfig) (string, error) {
	var interpolationErr error
	result := targetPlaceholderPattern.ReplaceAllStringFunc(target, func(token string) string {
		name := token[1 : len(token)-1]
		if value, ok := params[name]; ok {
			return value
		}
		if name == "git.username" && global != nil && global.Git.Username != "" {
			return global.Git.Username
		}
		interpolationErr = fmt.Errorf("placeholder %q has no value", name)
		return token
	})
	if interpolationErr != nil {
		return "", interpolationErr
	}
	return result, nil
}

func ParamAllowed(rule ParamRule, value string) bool {
	if len(rule.Allow) == 0 {
		return true
	}
	for _, allowed := range rule.Allow {
		if value == allowed {
			return true
		}
	}
	return false
}

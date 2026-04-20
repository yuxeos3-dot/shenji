package api

// translateBindErrors converts Gin's validator.ValidationErrors into a
// flat map[json_field] = user-facing Chinese message. The handler then
// can pick the first message for the toplevel "error" key AND return
// "fields" so the frontend can highlight the offending input inline.
//
// Why this exists: Gin's default err.Error() produces text like
//   "Key: 'CreateUserRequest.Email' Error:Field validation for
//    'Email' failed on the 'email' tag"
// which is fine for API debugging and actively hostile to end users.
// The screenshots showed this raw string being toast-bombed in the UI.

import (
	"reflect"
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"
)

// fieldRule defines human-readable messages keyed by the validator
// tag that was violated. Each handler provides its own map — we
// intentionally don't share a global table, because the same tag
// (e.g. "min") means different things for different fields.
type fieldRule struct {
	label    string // "姓名" / "邮箱" — unused now but handy for UI later
	required string // message when "required" tag fails
	email    string // message when "email" tag fails
	min      string // message when "min" tag fails
	max      string // message when "max" tag fails
}

// translateBindErrors inspects err. If it's a validator.ValidationErrors,
// each offending struct-field is mapped through the rules table and
// emitted as a lowercase-json-key entry. Anything we can't translate
// (non-validation bind errors, unknown fields) falls back under a
// magic "_" key so the caller always gets something.
func translateBindErrors(err error, rules map[string]fieldRule) map[string]string {
	out := map[string]string{}
	ve, ok := err.(validator.ValidationErrors)
	if !ok {
		// JSON parse errors, missing body, etc. — no field is identifiable.
		out["_"] = "参数错误: " + err.Error()
		return out
	}
	for _, fe := range ve {
		// fe.Field() returns the Go struct field name ("Email"), which
		// matches our rules map key. We also want the JSON tag so the
		// frontend doesn't need to know Go naming. Use a fallback
		// lowercasing when the struct type isn't available (shouldn't
		// happen in practice but keeps us safe).
		rule, haveRule := rules[fe.Field()]
		jsonKey := jsonTagFromField(fe.StructField())
		if jsonKey == "" {
			jsonKey = strings.ToLower(fe.Field())
		}
		msg := ""
		if haveRule {
			switch fe.Tag() {
			case "required":
				msg = rule.required
			case "email":
				msg = rule.email
			case "min":
				msg = rule.min
			case "max":
				msg = rule.max
			}
		}
		if msg == "" {
			// Unknown tag / no rule. Generic but still Chinese.
			if rule.label != "" {
				msg = rule.label + " 不符合要求"
			} else {
				msg = "字段不符合要求"
			}
		}
		out[jsonKey] = msg
	}
	return out
}

// firstFieldError returns a single message suitable for the top-level
// "error" key. Falls back to the given default.
func firstFieldError(fields map[string]string, fallback string) string {
	// Prefer deterministic order for repeatable UX — email, password,
	// name are the common triggers; anything not listed still returns
	// the first iteration. Map iteration is random in Go, so we check
	// a short priority list first.
	for _, k := range []string{"email", "password", "name"} {
		if v, ok := fields[k]; ok {
			return v
		}
	}
	for _, v := range fields {
		return v
	}
	return fallback
}

// jsonTagFromField is a tiny helper — validator.FieldError.StructField()
// gives us the reflect.StructField of the offending field, so we can
// read the json tag directly. Returns empty string if no tag.
//
// validator v10 exposes StructField() as a string (field name, not the
// reflect type), so we can't actually get the tag from it. We use a
// registry built via reflection at init time to translate.
func jsonTagFromField(name string) string {
	jsonNameCacheOnce.Do(initJSONNameCache)
	return jsonNameCache[name]
}

// Reflection cache: Go struct field name -> json tag key.
// Populated once at startup from all known request types.
var (
	jsonNameCache     = map[string]string{}
	jsonNameCacheOnce sync.Once
)

func initJSONNameCache() {
	// Register request types whose fields we want auto-translated.
	// Adding a new type here is the only maintenance step when a new
	// struct gains binding validations that should flow back as
	// "fields" to the UI.
	registerForJSONCache(reflect.TypeOf(CreateUserRequest{}))
	registerForJSONCache(reflect.TypeOf(RegisterRequest{}))
	registerForJSONCache(reflect.TypeOf(ApproveUserRequest{}))
}

func registerForJSONCache(t reflect.Type) {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		// Strip ",omitempty" etc.
		if idx := strings.Index(tag, ","); idx > 0 {
			tag = tag[:idx]
		}
		// Last write wins on collision — fine since we want the same
		// json name for a given Go name across request types.
		jsonNameCache[f.Name] = tag
	}
}

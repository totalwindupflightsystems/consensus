package opencode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ============================================================================
// ROUTE-ADD-071 — POST /experimental/project/{projectID}/copy/generate-name
// (experimental.projectCopy.generateName)
// ============================================================================

// projectCopyGenerateNameMaxWords bounds how many context words name the slug:
// the upstream operation promises a SHORT name ("Generate a short name for a
// project copy from task context"), and its sibling generator answers upstream's
// two-word Slug.create form, so two derived words is the same shape.
const projectCopyGenerateNameMaxWords = 2

// projectCopyGenerateNameMaxWordLen bounds one derived word so a single long
// token from the task context cannot produce a directory-hostile name.
const projectCopyGenerateNameMaxWordLen = 24

// projectCopyGenerateNameStopWords are the context words that carry no naming
// signal: articles, prepositions, auxiliaries, and the imperative verbs that
// open a task sentence ("fix the …", "add …", "update …"). They are skipped
// when deriving the slug so the name describes the subject of the task rather
// than its verb; a context made ONLY of them falls back to the generated
// two-word slug.
var projectCopyGenerateNameStopWords = map[string]struct{}{
	"a": {}, "an": {}, "and": {}, "the": {}, "to": {}, "of": {}, "for": {},
	"in": {}, "into": {}, "on": {}, "onto": {}, "at": {}, "by": {}, "with": {},
	"from": {}, "as": {}, "is": {}, "are": {}, "was": {}, "were": {}, "be": {},
	"been": {}, "being": {}, "it": {}, "its": {}, "this": {}, "that": {},
	"these": {}, "those": {}, "we": {}, "you": {}, "i": {}, "should": {},
	"must": {}, "will": {}, "can": {}, "please": {}, "then": {}, "than": {},
	"so": {}, "or": {}, "if": {}, "when": {}, "while": {}, "after": {},
	"before": {}, "via": {}, "per": {},
	// Imperative task verbs: the subject of the task names the copy, not the
	// request's opening verb.
	"add": {}, "build": {}, "change": {}, "create": {}, "delete": {},
	"ensure": {}, "fix": {}, "handle": {}, "implement": {}, "improve": {},
	"make": {}, "refactor": {}, "remove": {}, "run": {}, "support": {},
	"test": {}, "update": {}, "use": {},
}

// handleExperimentalProjectCopyGenerateName serves POST
// /experimental/project/{projectID}/copy/generate-name — upstream
// experimental.projectCopy.generateName (ROUTE-ADD-071; declared responses: 200
// {name: string} and 400 BadRequestError). Before this route was registered,
// net/http answered the default 404 for every request on the path (the
// declared-vs-served artifact's NOT-SERVED class, source item SHIM-DRIFT-002 in
// the pre-change artifact).
//
// ch:trace row=ROUTE-ADD-071 spec=specs/024-shim-support-matrix.md test=internal/shim/opencode/experimental_project_copy_generate_name_test.go doc=specs/openapi/upstream/consensus-shim-served-surface.yaml evidence=specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json witness=none:worker-local
//
// Why the declared 200 IS truthful here. The operation's declared contract is
// {name: string} — a NAME, not a stored copy, not a directory, not an id. The
// pinned upstream runtime answers it by asking its name generator for a slug
// from the task context; nothing in the declared response claims a project-copy
// registry row, a filesystem effect, or a model's reasoning. This handler
// therefore derives a name from the request's own input and returns it:
//
//   - a `context` that carries non-stop-word tokens yields a deterministic slug
//     of up to two of them (lowercased, alphanumeric, hyphen-joined, each token
//     bounded) — the same "short name from task context" the operation
//     describes, computed on this host with no model call and no stored state;
//   - an absent, empty or stop-word-only `context` yields the sibling copy
//     handler's two-word generator (projectCopySlug, upstream's Slug.create
//     form), because the operation has nothing to derive a name from and the
//     declared 200 still requires a name.
//
// Nothing is fabricated: no copy is created, no destination is claimed, and the
// name is not asserted to be unused — the collision suffixing belongs to the
// sibling v2.projectCopy.create, whose declared response reports the real
// created directory. This operation's answer is exactly the string it derived.
//
// Declared arms:
//
//   - present-but-blank {projectID} → the declared 400 BadRequestError
//     (data.kind "Params" — the pinned document declares `projectID` as a plain
//     required string with NO pattern, so blank is the only ill-shaped id the
//     contract itself defines; BadRequestError's kind enum carries Params for
//     request parameters, not the v2 InvalidRequestError's Path kind);
//   - a present-but-blank declared `directory`/`workspace` query selector → the
//     declared 400 BadRequestError (kind Query, the shared sibling validation
//     the /experimental/workspace family uses — the operation declares the same
//     two optional string selectors);
//   - a requestBody that is not exactly one JSON object carrying only the
//     declared optional `context` string → the declared 400 BadRequestError
//     (kind Body for malformed/non-object/multi-value/null bodies, kind Payload
//     for a `context` that is not a string). The document marks no
//     requestBody.required, so an absent or empty body is well-formed input;
//   - a name generator failure (crypto/rand unavailable) → the declared 400,
//     naming the failure. The operation declares no 5xx arm, so an undeclared
//     5xx is never emitted.
//
// Auth is untouched and unchanged: /experimental/* is not a stub path, so the
// standard api-key wall applies. experimental.projectCopy.generateName is a v1
// (experimental.*) operation, NOT a v2.* one, so the unauthenticated answer is
// the shim's own UNAUTHENTICATED envelope — the /experimental/project/<id>/copy
// shape rule in isV2EnvelopeRoutePath covers the /copy and POST create siblings
// only, exactly as it did before this route existed.
//
// Registration is EXACT (no /experimental/project/* catch-all): the sibling
// declared /copy paths keep their own contract rows, and undeclared deeper
// shapes keep the net/http default 404 they are classified with.
func (s *Server) handleExperimentalProjectCopyGenerateName(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}

	projectID := strings.TrimSpace(r.PathValue("projectID"))
	if projectID == "" {
		writeOpencodeBadRequest(w, r, "Params", "projectID must not be blank")
		return
	}

	// The operation declares the same optional directory/workspace string
	// selectors as the sibling /experimental/workspace family, so its declared
	// 400 arm answers a present-but-blank selector through the same rule.
	if !validateProjectCopyGenerateNameSelectors(w, r) {
		return
	}

	context, ok := validateProjectCopyGenerateNameBody(w, r)
	if !ok {
		return
	}

	name, err := projectCopyNameFromContext(context)
	if err != nil {
		writeOpencodeBadRequest(w, r, "",
			"could not generate a project copy name: "+err.Error())
		return
	}

	writeJSON(w, map[string]any{"name": name})
}

// validateProjectCopyGenerateNameSelectors validates the operation's two
// declared optional query selectors (`directory`, `workspace`, both plain
// strings). A present-but-blank selector is real malformed input and answers the
// declared 400 BadRequestError with data.kind "Query" — the same rule the
// sibling /experimental/workspace family applies, emitted here through the
// declared BadRequest shape (which carries `kind`) rather than the sibling's
// message-only body.
func validateProjectCopyGenerateNameSelectors(w http.ResponseWriter, r *http.Request) bool {
	q := r.URL.Query()
	for _, param := range []string{"directory", "workspace"} {
		if values, present := q[param]; present && (len(values) == 0 || strings.TrimSpace(values[0]) == "") {
			writeOpencodeBadRequest(w, r, "Query",
				fmt.Sprintf("query parameter %q must not be blank", param))
			return false
		}
	}
	return true
}

// validateProjectCopyGenerateNameBody validates the declared requestBody of
// experimental.projectCopy.generateName — one JSON object carrying only the
// optional `context` string (additionalProperties:false) — and reports the
// context to derive the name from. The body itself is OPTIONAL (the pinned
// document sets no requestBody.required), so an absent or empty body is
// well-formed and yields an empty context; a JSON null is not an object and
// answers the declared 400 (kind Body), the ptyUpdate / sibling-copy rule that
// the declared schema is an object, not a nullable.
func validateProjectCopyGenerateNameBody(w http.ResponseWriter, r *http.Request) (string, bool) {
	if r.Body == nil {
		return "", true
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeOpencodeBadRequest(w, r, "Body", "request body could not be read")
		return "", false
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return "", true
	}
	if bytes.Equal(trimmed, []byte("null")) {
		writeOpencodeBadRequest(w, r, "Body", "request body must be a JSON object, not null")
		return "", false
	}

	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	var body struct {
		Context json.RawMessage `json:"context"`
	}
	if err := dec.Decode(&body); err != nil {
		writeOpencodeBadRequest(w, r, "Body",
			`request body must be a JSON object carrying only the declared field (context)`)
		return "", false
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		writeOpencodeBadRequest(w, r, "Body", "request body must contain exactly one JSON object")
		return "", false
	}
	if len(body.Context) == 0 {
		return "", true
	}
	// A JSON null unmarshals into a string without error but would be an
	// undeclared (nullable) member — the declared context is a string.
	if strings.TrimSpace(string(body.Context)) == "null" {
		writeOpencodeBadRequest(w, r, "Payload", `field "context" must be a string, not null`)
		return "", false
	}
	var context string
	if err := json.Unmarshal(body.Context, &context); err != nil {
		writeOpencodeBadRequest(w, r, "Payload", `field "context" must be a string`)
		return "", false
	}
	return context, true
}

// projectCopyNameFromContext derives the declared 200 {name} from the request's
// task context, falling back to the sibling copy handler's two-word generator
// when the context carries no usable word. Only the name generator can fail
// (crypto/rand), and the caller answers that as the declared 400 because this
// operation declares no 5xx arm.
func projectCopyNameFromContext(context string) (string, error) {
	if slug := projectCopySlugFromWords(context); slug != "" {
		return slug, nil
	}
	return projectCopySlug()
}

// projectCopySlugFromWords turns free-form task context into a short kebab-case
// name: lowercase alphanumeric tokens, stop words dropped, at most
// projectCopyGenerateNameMaxWords kept, each token bounded by
// projectCopyGenerateNameMaxWordLen. It returns "" when nothing usable remains
// (the caller then generates a slug instead of emitting an empty name).
//
// The derivation is deliberately local and deterministic: the shim has no
// upstream name generator to call, and a name is not state — deriving it from
// the caller's own words keeps the declared 200 truthful without inventing a
// model response or a registry entry.
func projectCopySlugFromWords(context string) string {
	words := make([]string, 0, projectCopyGenerateNameMaxWords)
	for _, token := range strings.FieldsFunc(strings.ToLower(context), func(r rune) bool {
		return !isProjectCopyNameRune(r)
	}) {
		if _, stop := projectCopyGenerateNameStopWords[token]; stop {
			continue
		}
		if len(token) > projectCopyGenerateNameMaxWordLen {
			token = token[:projectCopyGenerateNameMaxWordLen]
		}
		words = append(words, token)
		if len(words) == projectCopyGenerateNameMaxWords {
			break
		}
	}
	return strings.Join(words, "-")
}

// isProjectCopyNameRune reports whether r survives in a derived name: only
// lowercase ASCII letters and digits are kept, everything else separates tokens
// (the same character class kebab-case slugs use on every platform).
func isProjectCopyNameRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
}

// tree-sitter-alder/grammar.js
//
// A second, independent description of Alder's syntax. It mirrors internal/syntax (the Go
// parser), not book/A-grammar.md. Newlines, HTML text, raw text and strings come from
// src/scanner.c, which ports the Go lexer's rules. Policy: accept everything the compiler
// accepts, perhaps a little more; diagnostics are the compiler's job.

const PREC = {
  body: -1, // lambda and if/else bodies swallow everything to their right
  pipe: 1, // |>
  default: 2, // ??  right-associative, looser than || (parser_expr.go parseDefault)
  or: 3, // ||
  and: 4, // &&
  compare: 5, // == != < <= > >=  (non-associative in the compiler; accepted left here)
  add: 6, // + - ++
  mul: 7, // * / %
  app: 8, // f a b
  postfix: 9, // x.field
};

const sep1 = (rule, separator) => seq(rule, repeat(seq(separator, rule)));
const commaSep1 = (rule) => seq(sep1(rule, ','), optional(','));
const commaSep = (rule) => optional(commaSep1(rule));

// Keywords that the Go parser accepts as deploy field names (`requires:` and the rest).
const TRUE_KEYWORDS = ['brand', 'where', 'let', 'match', 'if', 'then', 'else', 'for', 'in', 'requires'];

// The atoms of `_simple` and their no-brace twin `_subject_simple`, from one list so the two
// cannot drift apart. `fieldAccess` is the field-access rule of the family.
const simpleAtoms = ($, fieldAccess) => [
  $.identifier,
  $.constructor,
  $.qualified_name,
  fieldAccess,
  $.integer,
  $.float,
  $.string,
  $.boolean,
  $.unit,
  $.hole,
  $.list,
  $.parenthesized_expression,
];

// What a field access may be taken of, apart from `{…}` (records, updates, blocks), which only
// the ordinary family allows.
const fieldAccessValues = ($, fieldAccess) => [
  $.identifier,
  $.qualified_name,
  fieldAccess,
  $.parenthesized_expression,
  $.hole,
];

// Expression rules come in two families built from this one definition. The ordinary family
// (prefix '') is used everywhere but a match subject. There the Go parser sets noBrace
// (parser_expr.go withBraces, canStartArg): an application argument never starts with `{`, so
// `match f x {` leaves `{` to the arms, while the subject is otherwise any expression
// (`match a |> f {`, `match -1 {`, `match if c then x else y {`). The '_subject' family's rules
// are hidden (`_subject_pipe_expr`) and aliased to the ordinary names, so both build the same
// trees. Parentheses, lists, braces and arms go back to the ordinary family, as
// withBraces(true) does.
//
// The '_default' family is a field's default (`a: Int = f x as "A"`). A page parameter's wire
// name follows it, so `as` ends an application there (parser_expr.go canStartArg under
// fieldDefault). Its own rules keep that `as` out of the ordinary family's parse states:
// sharing them made `as` a keyword inside every application, so `f (g as)` failed where the Go
// parser reads `as` as a name (#63). Parentheses, lists and braces go back to the ordinary
// family, as Parser.nested does. Arguments are the ordinary family's.
const expressionRules = (prefix) => {
  const key = (name) => (!prefix ? name : name.startsWith('_') ? prefix + name : `${prefix}_${name}`);
  // Every in-family reference must go through ref: a bare $.name silently uses the ordinary family.
  const ref = ($, name) => (!prefix || name.startsWith('_') ? $[key(name)] : alias($[key(name)], $[name]));
  const expr = ($) => ref($, '_expression');
  const binary = (precedence, operator) => ($) =>
    precedence(seq(field('left', expr($)), operator, field('right', expr($))));
  return {
    [key('_expression')]: ($) =>
      choice(
        ...[
          'pipe_expr',
          'default_expr',
          'or_expr',
          'and_expr',
          'comparison_expr',
          'additive_expr',
          'multiplicative_expr',
          'application',
          '_term',
        ].map((name) => ref($, name)),
      ),

    [key('pipe_expr')]: binary((r) => prec.left(PREC.pipe, r), '|>'),
    [key('default_expr')]: binary((r) => prec.right(PREC.default, r), '??'),
    [key('or_expr')]: binary((r) => prec.left(PREC.or, r), '||'),
    [key('and_expr')]: binary((r) => prec.left(PREC.and, r), '&&'),
    [key('comparison_expr')]: binary((r) => prec.left(PREC.compare, r), choice('==', '!=', '<', '<=', '>', '>=')),
    [key('additive_expr')]: binary((r) => prec.left(PREC.add, r), choice('+', '-', '++')),
    [key('multiplicative_expr')]: binary((r) => prec.left(PREC.mul, r), choice('*', '/', '%')),

    // Arguments sit on the function's line: a newline ends the application wherever the
    // grammar expects a separator (scanner.c). `if`, `match`, elements and negative numbers
    // are not arguments; a lambda is, and its body runs to the end.
    [key('application')]: ($) =>
      prec.left(PREC.app, seq(field('function', $._simple), repeat1(field('argument', ref($, '_argument'))))),

    // A term may start with `{` in both families; only arguments differ.
    [key('_term')]: ($) =>
      choice(
        $._simple,
        ref($, 'lambda'),
        $.record,
        $.record_update,
        $.block,
        $.negative_number,
        ref($, 'if_expr'),
        $.match_expr,
        $._html,
      ),
    [key('_argument')]: ($) =>
      prefix === '_subject'
        ? choice($._subject_simple, ref($, 'lambda'))
        : choice($._simple, $.lambda, $.record, $.record_update, $.block),

    [key('lambda')]: ($) =>
      prec.right(
        PREC.body,
        seq(
          '\\',
          repeat(
            choice(
              field('param', $.identifier),
              seq('(', field('param', $.identifier), ':', field('param_type', $._type), ')'),
            ),
          ),
          '->',
          field('body', expr($)),
        ),
      ),

    [key('if_expr')]: ($) =>
      prec.right(
        PREC.body,
        seq(
          'if',
          field('condition', expr($)),
          'then',
          field('consequence', expr($)),
          'else',
          field('alternative', expr($)),
        ),
      ),
  };
};

module.exports = grammar({
  name: 'alder',

  word: ($) => $.identifier,

  externals: ($) => [
    $._newline, // scanner.c scan_newline: a separator, not emitted before an operator-led line
    $.string, // scanner.c scan_string: one token including its {…} interpolations
    $.html_text, // scanner.c scan_html_text: element text up to `<`+letter, `{` or a comment line
    $.raw_text, // scanner.c scan_raw_text: <script>/<style> bodies
    $._raw_tag_name, // scanner.c scan_raw_tag_name: `script` or `style` right after `<`
    $._implicit_end_tag, // scanner.c scan_html_text: zero width, closes an element left open before a column-1 line
    $._error_sentinel, // never in a rule: valid only during error recovery
    $._scratch_start, // scanner.c scan_scratch_start: zero width, a file whose first word is not `module`
  ],

  extras: ($) => [/[ \t\r\n]/, $.comment],

  conflicts: ($) => [
    // `x : T` NEWLINE `x = …` is one value declaration; in a signature file the next line may
    // be another signature. Both stay alive until the token after the name (`:` or not).
    [$.value_decl],
  ],

  rules: {
    source_file: ($) =>
      choice(
        seq(optional($._newline), $.module_decl, repeat(seq($._newline, $._item)), optional($._newline)),
        $.scratch_file,
      ),

    // A scratch file (internal/scratch file.go ParseFile): no module line, an optional
    // `in Module` header first, then items and `> expr` watches. In a scratch file a `>` at
    // column 1 followed by a space starts a watch rather than continuing the line before
    // (scanner.c scan_newline). Continuation lines are indented, as for any item. Two
    // newlines never meet: with the header and items each optional on their own, the leading
    // and trailing newlines would, and a comment line between them (which ends a newline token)
    // would let the trailing one end the file before its header.
    scratch_file: ($) => {
      const items = sep1(choice($._item, $.watch), $._newline);
      return seq(
        $._scratch_start,
        optional($._newline),
        optional(
          seq(choice(seq($.scratch_header, optional(seq($._newline, items))), items), optional($._newline)),
        ),
      );
    },

    scratch_header: ($) => seq('in', field('module', $.module_name)),
    watch: ($) => seq('>', field('expr', $._expression)),

    _item: ($) => choice($.import_decl, $._declaration),

    module_decl: ($) => seq('module', field('name', $.module_name)),
    import_decl: ($) =>
      seq('import', field('module', $.module_name), optional(seq('as', field('alias', $.constructor)))),

    // Module, type and component names: dotted uppercase segments (`Auth.Login`).
    module_name: ($) => seq($.constructor, repeat(seq('.', $.constructor))),

    _declaration: ($) =>
      choice(
        $.value_decl,
        $.type_decl,
        $.page_decl,
        $.msg_decl,
        $.form_decl,
        $.table_decl,
        $.capability_decl,
        $.fake_decl,
        $.session_decl,
        $.login_decl,
        $.test_decl,
        $.deploy_decl,
        $.silo_decl,
        $.choose_decl,
      ),

    // ---- values -------------------------------------------------------------------------

    value_decl: ($) =>
      choice(
        prec.dynamic(1, seq(field('signature', $.signature), $._newline, field('binding', $.binding))),
        field('signature', $.signature),
        field('binding', $.binding),
      ),

    signature: ($) => seq(field('name', $._signature_name), ':', field('type', $._type)),

    binding: ($) =>
      seq(field('name', $._value_name), optional(field('params', $.parameters)), '=', field('body', $._expression)),

    parameters: ($) => repeat1($.identifier),

    // Components are uppercase values.
    _value_name: ($) => choice($.identifier, $.constructor),
    // Stdlib signature files name values after keywords (`fake : …`, `login : …`); the Go
    // parser reads them as values only when `:` follows (parser.go peekIsColon), never in a
    // binding, so `login = Page` stays a login declaration.
    _signature_name: ($) => choice($._value_name, alias(choice('where', 'fake', 'login'), $.identifier)),

    // ---- types --------------------------------------------------------------------------

    type_decl: ($) =>
      seq(
        'type',
        field('name', $.constructor),
        repeat(field('param', alias($.identifier, $.type_var))),
        optional(seq('=', field('body', choice($.brand_type, $.record_type, $.union_type)))),
      ),

    union_type: ($) => seq(optional('|'), sep1($.variant, '|')),
    variant: ($) => prec.right(seq(field('name', $.constructor), optional(field('argument', $._type_atom)))),

    brand_type: ($) =>
      prec.right(seq('brand', field('base', $.constructor), optional(seq('where', field('check', $._expression))))),

    _type: ($) => choice($.type_arrow, $._type_term),
    type_arrow: ($) => prec.right(seq(field('parameter', $._type_term), '->', field('result', $._type))),
    _type_term: ($) => choice($.type_app, $._type_atom),
    type_app: ($) => prec.left(seq(field('constructor', $.type_name), repeat1(field('argument', $._type_atom)))),
    _type_atom: ($) =>
      choice($.type_name, alias($.identifier, $.type_var), $.record_type, $.unit_type, $.parenthesized_type),
    type_name: ($) => prec.left(seq($.constructor, repeat(seq('.', $.constructor)))),
    unit_type: ($) => seq('(', ')'),
    parenthesized_type: ($) => seq('(', $._type, ')'),

    record_type: ($) => seq('{', commaSep($.field_decl), '}'),

    // One field shape for records, forms, tables, capabilities and the session: `via` is legal
    // only in the session, `as "Key"` (a query key, #55) only on page parameters and attributes
    // only in tables, which the compiler checks.
    field_decl: ($) =>
      seq(
        field('name', $.identifier),
        ':',
        field('type', $._type),
        optional(seq('via', field('via', $.constructor))),
        optional(seq('=', field('default', $._default_expression))),
        optional(seq('as', field('wire', $.string))),
        repeat(field('attribute', $.field_attribute)),
      ),

    // @id @created @unique @owner @cascade … ; @ref T ; @partition key ; @renamed "old"
    field_attribute: ($) =>
      prec.right(
        seq('@', field('name', $.identifier), optional(field('argument', choice($.type_name, $.identifier, $.string)))),
      ),

    // ---- declarations -------------------------------------------------------------------

    page_decl: ($) =>
      seq('page', field('path', $.string), field('params', $.record_type), optional(field('requires', $.requires_clause))),

    requires_clause: ($) =>
      seq('requires', sep1(choice(alias($.identifier, $.session_key), $.role_clause, $.silo_clause), ',')),
    role_clause: ($) => seq('role', sep1(field('role', $.constructor), '|')),
    silo_clause: ($) => seq('silo', field('key', $.identifier)),

    msg_decl: ($) =>
      seq(
        'msg',
        field('name', $.constructor),
        optional(field('params', $.record_type)),
        optional(seq('form', field('form', $.constructor))),
      ),

    form_decl: ($) => seq('form', field('name', $.constructor), '=', field('fields', $.record_type)),

    table_decl: ($) =>
      seq('table', field('name', $.constructor), repeat(field('attribute', $.field_attribute)), field('fields', $.record_type)),

    capability_decl: ($) => seq('capability', field('name', $.constructor), field('entries', $.record_type)),

    fake_decl: ($) => seq('fake', field('name', $.constructor), field('body', $.fake_body)),
    fake_body: ($) => seq('{', repeat(choice($._newline, $.table_decl, $.fake_entry)), '}'),
    fake_entry: ($) =>
      seq(field('name', $.identifier), optional(field('params', $.parameters)), '=', field('body', $._expression)),

    session_decl: ($) => seq('session', field('fields', $.record_type)),

    login_decl: ($) => seq('login', '=', field('page', $.module_name)),

    test_decl: ($) => seq('test', field('name', $.string), '=', field('body', $._expression)),

    deploy_decl: ($) => seq('deploy', field('body', $.deploy_record)),
    deploy_record: ($) => seq('{', commaSep($.deploy_field), '}'),
    deploy_field: ($) =>
      seq(
        field('name', choice($.identifier, alias(choice(...TRUE_KEYWORDS), $.identifier))),
        ':',
        field('value', $._deploy_value),
      ),
    _deploy_value: ($) => choice($.string, $.integer, $.float, $.negative_number, $.deploy_list, $.deploy_target),
    deploy_list: ($) => seq('[', commaSep($._deploy_value), ']'),
    deploy_target: ($) =>
      prec.right(
        seq(field('kind', $.constructor), optional(field('argument', $.string)), optional(field('options', $.deploy_record))),
      ),

    silo_decl: ($) => seq('silo', field('key', $.identifier), '{', commaSep(field('table', $.constructor)), '}'),

    choose_decl: ($) => seq('choose', field('key', $.identifier), '=', field('page', $.module_name)),

    // ---- expressions --------------------------------------------------------------------

    ...expressionRules(''),
    ...expressionRules('_subject'),
    ...expressionRules('_default'),

    _simple: ($) => choice(...simpleAtoms($, $.field_access)),
    // A no-brace argument never starts with `{` (canStartArg under noBrace), not even as the
    // record of a field access: `match f x {` must leave `{` to the arms.
    _subject_simple: ($) => choice(...simpleAtoms($, alias($._subject_field_access, $.field_access))),

    // `Db.from`, `Todo.TodoId`, `A.B.c`: a module segment then a value or constructor.
    qualified_name: ($) =>
      prec.right(
        seq(field('module', $.constructor), '.', field('name', choice($.identifier, $.constructor, $.qualified_name))),
      ),

    // `post.title`, `Post.slug.eq`, `(f x).y`; never on a bare constructor.
    field_access: ($) =>
      prec(
        PREC.postfix,
        seq(
          field('value', choice(...fieldAccessValues($, $.field_access), $.record, $.record_update, $.block)),
          '.',
          field('field', $.identifier),
        ),
      ),
    _subject_field_access: ($) =>
      prec(
        PREC.postfix,
        seq(
          field('value', choice(...fieldAccessValues($, alias($._subject_field_access, $.field_access)))),
          '.',
          field('field', $.identifier),
        ),
      ),

    // The subject is a no-brace expression, so `match f x {` opens the arms.
    match_expr: ($) => seq('match', field('subject', $._subject_expression), '{', commaSep($.arm), '}'),
    arm: ($) => seq(field('pattern', $._pattern), '->', field('body', $._expression)),

    record: ($) => seq('{', commaSep($.field_init), '}'),
    field_init: ($) => seq(field('name', $.identifier), ':', field('value', $._expression)),
    record_update: ($) => seq('{', field('record', $._expression), '|', commaSep1($.field_init), '}'),

    block: ($) => seq('{', repeat(choice($.let_stmt, $.bind_stmt)), field('result', $._expression), '}'),
    let_stmt: ($) =>
      seq(
        'let',
        field('name', choice($.identifier, $.wildcard)),
        optional(seq(':', field('type', $._type))),
        '=',
        field('value', $._expression),
        $._newline,
      ),
    bind_stmt: ($) =>
      seq(
        'let',
        field('name', choice($.identifier, $.wildcard)),
        optional(seq(':', field('type', $._type))),
        '<-',
        field('value', $._expression),
        $._newline,
      ),

    list: ($) => seq('[', commaSep($._expression), ']'),
    parenthesized_expression: ($) => seq('(', $._expression, ')'),
    unit: ($) => seq('(', ')'),

    negative_number: ($) => seq('-', choice($.integer, $.float)),
    integer: ($) => /[0-9]+/,
    float: ($) => /[0-9]+(\.[0-9]+)?[eE][+-]?[0-9]+|[0-9]+\.[0-9]+/,
    boolean: ($) => choice('True', 'False'),
    hole: ($) => /\?[a-zA-Z0-9_]*/,

    identifier: ($) => /[a-z][a-zA-Z0-9_]*|_[a-zA-Z0-9_]+/,
    constructor: ($) => /[A-Z][a-zA-Z0-9_]*/,

    // ---- patterns -----------------------------------------------------------------------

    _pattern: ($) =>
      choice(
        $.wildcard,
        $.identifier,
        $.integer,
        $.negative_number,
        $.string,
        $.boolean,
        $.constructor_pattern,
        $.record_pattern,
        $.list_pattern,
        $.parenthesized_pattern,
      ),
    wildcard: ($) => '_',
    constructor_pattern: ($) =>
      prec.right(seq(field('constructor', $.constructor), optional(field('argument', $._pattern_argument)))),
    _pattern_argument: ($) =>
      choice(
        $.wildcard,
        $.identifier,
        $.integer,
        $.string,
        $.boolean,
        alias($._bare_constructor_pattern, $.constructor_pattern),
        $.record_pattern,
        $.parenthesized_pattern,
      ),
    _bare_constructor_pattern: ($) => field('constructor', $.constructor),
    record_pattern: ($) => seq('{', commaSep($.field_pattern), '}'),
    field_pattern: ($) => seq(field('name', $.identifier), ':', field('pattern', $._pattern)),
    list_pattern: ($) =>
      seq(
        '[',
        optional(choice(seq(sep1($._pattern, ','), optional(seq(',', optional($.rest_pattern)))), $.rest_pattern)),
        ']',
      ),
    rest_pattern: ($) => seq('..', field('name', $.identifier)),
    parenthesized_pattern: ($) => seq('(', $._pattern, ')'),

    // ---- HTML ---------------------------------------------------------------------------

    _html: ($) => choice($.element, $.self_closing_element, $.raw_element),

    // An element left open before a column-1 line (code being typed) ends there, so the
    // declaration on that line still parses as one (scanner.c scan_html_text). The compiler
    // rejects such code; the tree accepts a little more.
    element: ($) =>
      seq(
        field('open', $.start_tag),
        repeat($._html_child),
        choice(field('close', $.end_tag), $._implicit_end_tag),
      ),
    start_tag: ($) => seq('<', field('tag', $._tag_name), repeat($.attribute), '>'),
    end_tag: ($) => seq('</', field('tag', $._tag_name), '>'),
    self_closing_element: ($) => seq('<', field('tag', $._tag_name), repeat($.attribute), '/', '>'),

    raw_element: ($) =>
      choice(
        seq(
          '<',
          field('tag', alias($._raw_tag_name, $.tag_name)),
          repeat($.attribute),
          '>',
          optional($.raw_text),
          '</',
          field('close', $._tag_name),
          '>',
        ),
        seq('<', field('tag', alias($._raw_tag_name, $.tag_name)), repeat($.attribute), '/', '>'),
      ),

    // Lowercase names may contain `-` (custom elements) and may be keywords (`<table>`,
    // `<form>`): this token is not the word token, so keywords are never extracted from it.
    _tag_name: ($) => choice(alias($._html_name, $.tag_name), alias($.module_name, $.component_name)),
    _html_name: ($) => /[a-z][a-zA-Z0-9_-]*/,

    attribute: ($) =>
      seq(
        field('name', alias($._html_name, $.attribute_name)),
        optional(seq('=', field('value', choice($.string, $.interpolation)))),
      ),

    _html_child: ($) => choice($.html_text, $._html, $.interpolation, $.html_for, $.html_if, $.html_match),

    interpolation: ($) => seq('{', field('expression', $._expression), '}'),

    // `for`, `if` and `match` are special only as the first token inside a content brace.
    html_for: ($) =>
      seq(
        '{',
        'for',
        field('binder', $.identifier),
        'in',
        field('items', $._expression),
        '->',
        field('body', $._expression),
        '}',
      ),
    html_if: ($) =>
      prec(
        1,
        seq(
          '{',
          'if',
          field('condition', $._expression),
          'then',
          field('consequence', $._expression),
          'else',
          field('alternative', $._expression),
          '}',
        ),
      ),
    html_match: ($) => prec(1, seq('{', 'match', field('subject', $._subject_expression), '{', commaSep($.arm), '}', '}')),

    comment: ($) => token(seq('--', /.*/)),
  },
});

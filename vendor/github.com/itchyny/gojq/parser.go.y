%{
package gojq

func reverseFuncDef(xs []*FuncDef) []*FuncDef {
	for i, j := 0, len(xs)-1; i < j; i, j = i+1, j-1 {
		xs[i], xs[j] = xs[j], xs[i]
	}
	return xs
}

func prependFuncDef(xs []*FuncDef, x *FuncDef) []*FuncDef {
	xs = append(xs, nil)
	copy(xs[1:], xs)
	xs[0] = x
	return xs
}
%}

%union {
  value    any
  token    string
  operator Operator
}

%type<value> program header imports import meta body funcdefs funcdef funcargs query
%type<value> bindpatterns pattern arraypatterns objectpatterns objectpattern
%type<value> expr term string stringparts suffix args ifelifs ifelse trycatch
%type<value> objectkeyvals objectkeyval objectval
%type<value> constterm constobject constobjectkeyvals constobjectkeyval constarray constarrayelems
%type<token> tokIdentVariable tokIdentModuleIdent tokVariableModuleVariable tokKeyword objectkey
%token<operator> tokAltOp tokUpdateOp tokDestAltOp tokCompareOp
%token<token> tokOrOp tokAndOp tokModule tokImport tokInclude tokDef tokAs tokLabel tokBreak
%token<token> tokNull tokTrue tokFalse
%token<token> tokIf tokThen tokElif tokElse tokEnd
%token<token> tokTry tokCatch tokReduce tokForeach
%token<token> tokIdent tokVariable tokModuleIdent tokModuleVariable
%token<token> tokRecurse tokIndex tokNumber tokFormat
%token<token> tokString tokStringStart tokStringQuery tokStringEnd
%token<token> tokInvalid tokInvalidEscapeSequence tokUnterminatedString

%nonassoc tokFuncDefQuery tokExpr tokTerm
%right '|'
%left ','
%right tokAltOp
%nonassoc tokUpdateOp
%left tokOrOp
%left tokAndOp
%nonassoc tokCompareOp
%left '+' '-'
%left '*' '/' '%'
%nonassoc tokAs tokIndex '.' '?' tokEmptyCatch
%nonassoc '[' tokTry tokCatch

%%

program
    : header imports body
    {
        query := $3.(*Query)
        query.Meta = $1.(*ConstObject)
        query.Imports = $2.([]*Import)
        yylex.(*lexer).result = query
    }

header
    :
    {
        $$ = (*ConstObject)(nil)
    }
    | tokModule constobject ';'
    {
        $$ = $2;
    }

imports
    :
    {
        $$ = []*Import(nil)
    }
    | imports import
    {
        $$ = append($1.([]*Import), $2.(*Import))
    }

import
    : tokImport tokString tokAs tokIdentVariable meta ';'
    {
        $$ = &Import{ImportPath: $2, ImportAlias: $4, Meta: $5.(*ConstObject)}
    }
    | tokInclude tokString meta ';'
    {
        $$ = &Import{IncludePath: $2, Meta: $3.(*ConstObject)}
    }

meta
    :
    {
        $$ = (*ConstObject)(nil)
    }
    | constobject

body
    : funcdefs
    {
        $$ = &Query{FuncDefs: reverseFuncDef($1.([]*FuncDef))}
    }
    | query

funcdefs
    :
    {
        $$ = []*FuncDef(nil)
    }
    | funcdef funcdefs
    {
        $$ = append($2.([]*FuncDef), $1.(*FuncDef))
    }

funcdef
    : tokDef tokIdent ':' query ';'
    {
        $$ = &FuncDef{Name: $2, Body: $4.(*Query)}
    }
    | tokDef tokIdent '(' funcargs ')' ':' query ';'
    {
        $$ = &FuncDef{$2, $4.([]string), $7.(*Query)}
    }

funcargs
    : tokIdentVariable
    {
        $$ = []string{$1}
    }
    | funcargs ';' tokIdentVariable
    {
        $$ = append($1.([]string), $3)
    }

tokIdentVariable
    : tokIdent
    | tokVariable

query
    : funcdef query %prec tokFuncDefQuery
    {
        query := $2.(*Query)
        query.FuncDefs = prependFuncDef(query.FuncDefs, $1.(*FuncDef))
        $$ = query
    }
    | query '|' query
    {
        $$ = &Query{Left: $1.(*Query), Op: OpPipe, Right: $3.(*Query)}
    }
    | term tokAs bindpatterns '|' query
    {
        term := $1.(*Term)
        term.SuffixList = append(term.SuffixList, &Suffix{Bind: &Bind{$3.([]*Pattern), $5.(*Query)}})
        $$ = &Query{Term: term}
    }
    | tokLabel tokVariable '|' query
    {
        $$ = &Query{Term: &Term{Type: TermTypeLabel, Label: &Label{$2, $4.(*Query)}}}
    }
    | query ',' query
    {
        $$ = &Query{Left: $1.(*Query), Op: OpComma, Right: $3.(*Query)}
    }
    | expr %prec tokExpr

expr
    : expr tokAltOp expr
    {
        $$ = &Query{Left: $1.(*Query), Op: $2, Right: $3.(*Query)}
    }
    | expr tokUpdateOp expr
    {
        $$ = &Query{Left: $1.(*Query), Op: $2, Right: $3.(*Query)}
    }
    | expr tokOrOp expr
    {
        $$ = &Query{Left: $1.(*Query), Op: OpOr, Right: $3.(*Query)}
    }
    | expr tokAndOp expr
    {
        $$ = &Query{Left: $1.(*Query), Op: OpAnd, Right: $3.(*Query)}
    }
    | expr tokCompareOp expr
    {
        $$ = &Query{Left: $1.(*Query), Op: $2, Right: $3.(*Query)}
    }
    | expr '+' expr
    {
        $$ = &Query{Left: $1.(*Query), Op: OpAdd, Right: $3.(*Query)}
    }
    | expr '-' expr
    {
        $$ = &Query{Left: $1.(*Query), Op: OpSub, Right: $3.(*Query)}
    }
    | expr '*' expr
    {
        $$ = &Query{Left: $1.(*Query), Op: OpMul, Right: $3.(*Query)}
    }
    | expr '/' expr
    {
        $$ = &Query{Left: $1.(*Query), Op: OpDiv, Right: $3.(*Query)}
    }
    | expr '%' expr
    {
        $$ = &Query{Left: $1.(*Query), Op: OpMod, Right: $3.(*Query)}
    }
    | term %prec tokTerm
    {
        $$ = &Query{Term: $1.(*Term)}
    }

bindpatterns
    : pattern
    {
        $$ = []*Pattern{$1.(*Pattern)}
    }
    | bindpatterns tokDestAltOp pattern
    {
        $$ = append($1.([]*Pattern), $3.(*Pattern))
    }

pattern
    : tokVariable
    {
        $$ = &Pattern{Name: $1}
    }
    | '[' arraypatterns ']'
    {
        $$ = &Pattern{Array: $2.([]*Pattern)}
    }
    | '{' objectpatterns '}'
    {
        $$ = &Pattern{Object: $2.([]*PatternObject)}
    }

arraypatterns
    : pattern
    {
        $$ = []*Pattern{$1.(*Pattern)}
    }
    | arraypatterns ',' pattern
    {
        $$ = append($1.([]*Pattern), $3.(*Pattern))
    }

objectpatterns
    : objectpattern
    {
        $$ = []*PatternObject{$1.(*PatternObject)}
    }
    | objectpatterns ',' objectpattern
    {
        $$ = append($1.([]*PatternObject), $3.(*PatternObject))
    }

objectpattern
    : objectkey ':' pattern
    {
        $$ = &PatternObject{Key: $1, Val: $3.(*Pattern)}
    }
    | string ':' pattern
    {
        $$ = &PatternObject{KeyString: $1.(*String), Val: $3.(*Pattern)}
    }
    | '(' query ')' ':' pattern
    {
        $$ = &PatternObject{KeyQuery: $2.(*Query), Val: $5.(*Pattern)}
    }
    | tokVariable
    {
        $$ = &PatternObject{Key: $1}
    }

term
    : '.'
    {
        $$ = &Term{Type: TermTypeIdentity}
    }
    | tokRecurse
    {
        $$ = &Term{Type: TermTypeRecurse}
    }
    | tokIndex
    {
        $$ = &Term{Type: TermTypeIndex, Index: &Index{Name: $1}}
    }
    | '.' suffix
    {
        suffix := $2.(*Suffix)
        if suffix.Iter {
            $$ = &Term{Type: TermTypeIdentity, SuffixList: []*Suffix{suffix}}
        } else {
            $$ = &Term{Type: TermTypeIndex, Index: suffix.Index}
        }
    }
    | '.' string
    {
        $$ = &Term{Type: TermTypeIndex, Index: &Index{Str: $2.(*String)}}
    }
    | tokNull
    {
        $$ = &Term{Type: TermTypeNull}
    }
    | tokTrue
    {
        $$ = &Term{Type: TermTypeTrue}
    }
    | tokFalse
    {
        $$ = &Term{Type: TermTypeFalse}
    }
    | tokIdentModuleIdent
    {
        $$ = &Term{Type: TermTypeFunc, Func: &Func{Name: $1}}
    }
    | tokIdentModuleIdent '(' args ')'
    {
        $$ = &Term{Type: TermTypeFunc, Func: &Func{Name: $1, Args: $3.([]*Query)}}
    }
    | tokVariableModuleVariable
    {
        $$ = &Term{Type: TermTypeFunc, Func: &Func{Name: $1}}
    }
    | '{' '}'
    {
        $$ = &Term{Type: TermTypeObject, Object: &Object{}}
    }
    | '{' objectkeyvals '}'
    {
        $$ = &Term{Type: TermTypeObject, Object: &Object{$2.([]*ObjectKeyVal)}}
    }
    | '{' objectkeyvals ',' '}'
    {
        $$ = &Term{Type: TermTypeObject, Object: &Object{$2.([]*ObjectKeyVal)}}
    }
    | '[' ']'
    {
        $$ = &Term{Type: TermTypeArray, Array: &Array{}}
    }
    | '[' query ']'
    {
        $$ = &Term{Type: TermTypeArray, Array: &Array{$2.(*Query)}}
    }
    | tokNumber
    {
        $$ = &Term{Type: TermTypeNumber, Number: $1}
    }
    | '+' term
    {
        $$ = &Term{Type: TermTypeUnary, Unary: &Unary{OpAdd, $2.(*Term)}}
    }
    | '-' term
    {
        $$ = &Term{Type: TermTypeUnary, Unary: &Unary{OpSub, $2.(*Term)}}
    }
    | tokFormat
    {
        $$ = &Term{Type: TermTypeFormat, Format: $1}
    }
    | tokFormat string
    {
        $$ = &Term{Type: TermTypeFormat, Format: $1, Str: $2.(*String)}
    }
    | string
    {
        $$ = &Term{Type: TermTypeString, Str: $1.(*String)}
    }
    | tokIf query tokThen query ifelifs ifelse tokEnd
    {
        $$ = &Term{Type: TermTypeIf, If: &If{$2.(*Query), $4.(*Query), $5.([]*IfElif), $6.(*Query)}}
    }
    | tokTry expr trycatch
    {
        $$ = &Term{Type: TermTypeTry, Try: &Try{$2.(*Query), $3.(*Query)}}
    }
    | tokReduce expr tokAs pattern '(' query ';' query ')'
    {
        $$ = &Term{Type: TermTypeReduce, Reduce: &Reduce{$2.(*Query), $4.(*Pattern), $6.(*Query), $8.(*Query)}}
    }
    | tokForeach expr tokAs pattern '(' query ';' query ')'
    {
        $$ = &Term{Type: TermTypeForeach, Foreach: &Foreach{$2.(*Query), $4.(*Pattern), $6.(*Query), $8.(*Query), nil}}
    }
    | tokForeach expr tokAs pattern '(' query ';' query ';' query ')'
    {
        $$ = &Term{Type: TermTypeForeach, Foreach: &Foreach{$2.(*Query), $4.(*Pattern), $6.(*Query), $8.(*Query), $10.(*Query)}}
    }
    | tokBreak tokVariable
    {
        $$ = &Term{Type: TermTypeBreak, Break: $2}
    }
    | '(' query ')'
    {
        $$ = &Term{Type: TermTypeQuery, Query: $2.(*Query)}
    }
    | term tokIndex
    {
        $1.(*Term).SuffixList = append($1.(*Term).SuffixList, &Suffix{Index: &Index{Name: $2}})
    }
    | term suffix
    {
        $1.(*Term).SuffixList = append($1.(*Term).SuffixList, $2.(*Suffix))
    }
    | term '?'
    {
        $1.(*Term).SuffixList = append($1.(*Term).SuffixList, &Suffix{Optional: true})
    }
    | term '.' suffix
    {
        $1.(*Term).SuffixList = append($1.(*Term).SuffixList, $3.(*Suffix))
    }
    | term '.' string
    {
        $1.(*Term).SuffixList = append($1.(*Term).SuffixList, &Suffix{Index: &Index{Str: $3.(*String)}})
    }

string
    : tokString
    {
        $$ = &String{Str: $1}
    }
    | tokStringStart stringparts tokStringEnd
    {
        $$ = &String{Queries: $2.([]*Query)}
    }

stringparts
    :
    {
        $$ = []*Query{}
    }
    | stringparts tokString
    {
        $$ = append($1.([]*Query), &Query{Term: &Term{Type: TermTypeString, Str: &String{Str: $2}}})
    }
    | stringparts tokStringQuery query ')'
    {
        yylex.(*lexer).inString = true
        $$ = append($1.([]*Query), &Query{Term: &Term{Type: TermTypeQuery, Query: $3.(*Query)}})
    }

tokIdentModuleIdent
    : tokIdent
    | tokModuleIdent

tokVariableModuleVariable
    : tokVariable
    | tokModuleVariable

suffix
    : '[' ']'
    {
        $$ = &Suffix{Iter: true}
    }
    | '[' query ']'
    {
        $$ = &Suffix{Index: &Index{Start: $2.(*Query)}}
    }
    | '[' query ':' ']'
    {
        $$ = &Suffix{Index: &Index{Start: $2.(*Query), IsSlice: true}}
    }
    | '[' ':' query ']'
    {
        $$ = &Suffix{Index: &Index{End: $3.(*Query), IsSlice: true}}
    }
    | '[' query ':' query ']'
    {
        $$ = &Suffix{Index: &Index{Start: $2.(*Query), End: $4.(*Query), IsSlice: true}}
    }

args
    : query
    {
        $$ = []*Query{$1.(*Query)}
    }
    | args ';' query
    {
        $$ = append($1.([]*Query), $3.(*Query))
    }

ifelifs
    :
    {
        $$ = []*IfElif(nil)
    }
    | ifelifs tokElif query tokThen query
    {
        $$ = append($1.([]*IfElif), &IfElif{$3.(*Query), $5.(*Query)})
    }

ifelse
    :
    {
        $$ = (*Query)(nil)
    }
    | tokElse query
    {
        $$ = $2
    }

trycatch
    : %prec tokEmptyCatch
    {
        $$ = (*Query)(nil)
    }
    | tokCatch expr
    {
        $$ = $2
    }

objectkeyvals
    : objectkeyval
    {
        $$ = []*ObjectKeyVal{$1.(*ObjectKeyVal)}
    }
    | objectkeyvals ',' objectkeyval
    {
        $$ = append($1.([]*ObjectKeyVal), $3.(*ObjectKeyVal))
    }

objectkeyval
    : objectkey ':' objectval
    {
        $$ = &ObjectKeyVal{Key: $1, Val: $3.(*Query)}
    }
    | string ':' objectval
    {
        $$ = &ObjectKeyVal{KeyString: $1.(*String), Val: $3.(*Query)}
    }
    | '(' query ')' ':' objectval
    {
        $$ = &ObjectKeyVal{KeyQuery: $2.(*Query), Val: $5.(*Query)}
    }
    | objectkey
    {
        $$ = &ObjectKeyVal{Key: $1}
    }
    | string
    {
        $$ = &ObjectKeyVal{KeyString: $1.(*String)}
    }

objectkey
    : tokIdent
    | tokVariable
    | tokKeyword

objectval
    : objectval '|' objectval
    {
        $$ = &Query{Left: $1.(*Query), Op: OpPipe, Right: $3.(*Query)}
    }
    | expr

constterm
    : constobject
    {
        $$ = &ConstTerm{Object: $1.(*ConstObject)}
    }
    | constarray
    {
        $$ = &ConstTerm{Array: $1.(*ConstArray)}
    }
    | tokNumber
    {
        $$ = &ConstTerm{Number: $1}
    }
    | tokString
    {
        $$ = &ConstTerm{Str: $1}
    }
    | tokNull
    {
        $$ = &ConstTerm{Null: true}
    }
    | tokTrue
    {
        $$ = &ConstTerm{True: true}
    }
    | tokFalse
    {
        $$ = &ConstTerm{False: true}
    }

constobject
    : '{' '}'
    {
        $$ = &ConstObject{}
    }
    | '{' constobjectkeyvals '}'
    {
        $$ = &ConstObject{$2.([]*ConstObjectKeyVal)}
    }
    | '{' constobjectkeyvals ',' '}'
    {
        $$ = &ConstObject{$2.([]*ConstObjectKeyVal)}
    }

constobjectkeyvals
    : constobjectkeyval
    {
        $$ = []*ConstObjectKeyVal{$1.(*ConstObjectKeyVal)}
    }
    | constobjectkeyvals ',' constobjectkeyval
    {
        $$ = append($1.([]*ConstObjectKeyVal), $3.(*ConstObjectKeyVal))
    }

constobjectkeyval
    : tokIdent ':' constterm
    {
        $$ = &ConstObjectKeyVal{Key: $1, Val: $3.(*ConstTerm)}
    }
    | tokKeyword ':' constterm
    {
        $$ = &ConstObjectKeyVal{Key: $1, Val: $3.(*ConstTerm)}
    }
    | tokString ':' constterm
    {
        $$ = &ConstObjectKeyVal{KeyString: $1, Val: $3.(*ConstTerm)}
    }

constarray
    : '[' ']'
    {
        $$ = &ConstArray{}
    }
    | '[' constarrayelems ']'
    {
        $$ = &ConstArray{$2.([]*ConstTerm)}
    }

constarrayelems
    : constterm
    {
        $$ = []*ConstTerm{$1.(*ConstTerm)}
    }
    | constarrayelems ',' constterm
    {
        $$ = append($1.([]*ConstTerm), $3.(*ConstTerm))
    }

tokKeyword
    : tokOrOp
    | tokAndOp
    | tokModule
    | tokImport
    | tokInclude
    | tokDef
    | tokAs
    | tokLabel
    | tokBreak
    | tokNull
    | tokTrue
    | tokFalse
    | tokIf
    | tokThen
    | tokElif
    | tokElse
    | tokEnd
    | tokTry
    | tokCatch
    | tokReduce
    | tokForeach

%%

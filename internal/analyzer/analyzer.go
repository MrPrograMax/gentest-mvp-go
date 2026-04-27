// Пакет analyzer извлекает FunctionSpec из загруженного Go-пакета.
//
// Классификация типов выполняется через go/types (а не строковым сравнением),
// что позволяет корректно обрабатывать псевдонимы, именованные интерфейсы
// и внешние пакеты. Строковое представление (TypeStr) сохраняется из AST
// — оно нужно генератору кода, а не type-checker'у.
//
// Дополнительно analyzeGuards обходит тело функции и собирает Guards-факты,
// которые используются scenario.Generate для построения осмысленных edge-сценариев.
package analyzer

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/yourorg/testgen/internal/loader"
	"github.com/yourorg/testgen/internal/model"
)

// Analyze возвращает FunctionSpec для каждой экспортируемой функции / метода
// из всех пакетов в r.
func Analyze(r *loader.Result) ([]model.FunctionSpec, error) {
	var specs []model.FunctionSpec

	for _, pkg := range r.Pkgs {
		for _, file := range pkg.Syntax {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || !fn.Name.IsExported() {
					continue
				}
				specs = append(specs, buildSpec(pkg, r.Dir, fn))
			}
		}
	}

	return specs, nil
}

// buildSpec преобразует *ast.FuncDecl в model.FunctionSpec.
// pkg передаётся для доступа к TypesInfo (type-checker).
func buildSpec(pkg *packages.Package, pkgPath string, fn *ast.FuncDecl) model.FunctionSpec {
	spec := model.FunctionSpec{
		PackageName:       pkg.Name,
		PackagePath:       pkgPath,
		PackageImportPath: pkg.PkgPath,
		Name:              fn.Name.Name,
	}

	// ── Получатель (receiver) ─────────────────────────────────────────────────
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		recv := fn.Recv.List[0]
		spec.IsMethod = true
		spec.ReceiverType = exprStr(recv.Type)
		if len(recv.Names) > 0 {
			spec.ReceiverName = recv.Names[0].Name
		} else {
			spec.ReceiverName = "rcv"
		}
		// Извлекаем поля receiver-структуры через go/types.
		// Нужны mockplan для поиска интерфейс-полей.
		spec.ReceiverFields = receiverFields(pkg, recv.Type)
	}

	// ── Параметры ─────────────────────────────────────────────────────────────
	if fn.Type.Params != nil {
		idx := 0
		for _, field := range fn.Type.Params.List {
			// Вариадический параметр: AST использует *ast.Ellipsis для последнего поля.
			if _, ok := field.Type.(*ast.Ellipsis); ok {
				spec.IsVariadic = true
			}
			ts := exprStr(field.Type)
			tsFull := qualifiedTypeStr(pkg.TypesInfo, field.Type)
			kind := classifyExpr(field.Type, pkg.TypesInfo)

			if len(field.Names) == 0 {
				// Анонимный параметр.
				spec.Params = append(spec.Params, model.ParamSpec{
					Name:        fmt.Sprintf("arg%d", idx),
					TypeStr:     ts,
					TypeStrFull: tsFull,
					Kind:        kind,
				})
				idx++
			} else {
				for _, name := range field.Names {
					spec.Params = append(spec.Params, model.ParamSpec{
						Name:        name.Name,
						TypeStr:     ts,
						TypeStrFull: tsFull,
						Kind:        kind,
					})
					idx++
				}
			}
		}
	}

	// ── Результаты ────────────────────────────────────────────────────────────
	if fn.Type.Results != nil {
		idx := 0
		for _, field := range fn.Type.Results.List {
			ts := exprStr(field.Type)
			tsFull := qualifiedTypeStr(pkg.TypesInfo, field.Type)
			kind := classifyExpr(field.Type, pkg.TypesInfo)
			isErr := kind == model.KindError

			name := fmt.Sprintf("result%d", idx)
			if isErr {
				name = "err"
			}
			if len(field.Names) > 0 {
				name = field.Names[0].Name
			}

			spec.Results = append(spec.Results, model.ParamSpec{
				Name:        name,
				TypeStr:     ts,
				TypeStrFull: tsFull,
				Kind:        kind,
				IsError:     isErr,
			})
			if isErr {
				spec.HasError = true
			}
			idx++
		}
	}

	// ── Анализ тела функции (Guards) ──────────────────────────────────────────
	if fn.Body != nil && pkg.TypesInfo != nil {
		spec.Guards = analyzeGuards(fn, pkg.TypesInfo)
	} else {
		// Нет тела или нет типовой информации — инициализируем пустыми картами.
		spec.Guards = model.Guards{
			NilCheckedParams:   make(map[string]bool),
			EmptyCheckedParams: make(map[string]bool),
		}
	}

	return spec
}

// ── Анализ Guards ─────────────────────────────────────────────────────────────

// analyzeGuards обходит тело функции и собирает Guards-факты.
func analyzeGuards(fn *ast.FuncDecl, info *types.Info) model.Guards {
	g := model.Guards{
		NilCheckedParams:   make(map[string]bool),
		EmptyCheckedParams: make(map[string]bool),
	}

	// Собираем имена параметров для быстрой проверки.
	paramNames := make(map[string]bool)
	if fn.Type.Params != nil {
		for _, field := range fn.Type.Params.List {
			for _, name := range field.Names {
				paramNames[name.Name] = true
			}
		}
	}

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			// Проверяем вызовы panic().
			if ident, ok := node.Fun.(*ast.Ident); ok && ident.Name == "panic" {
				g.HasPanic = true
			}

		case *ast.BinaryExpr:
			if node.Op != token.EQL && node.Op != token.NEQ {
				return true
			}
			checkNilComparison(node, paramNames, info, &g)
			checkEmptyStringComparison(node, paramNames, &g)
			checkLenComparison(node, paramNames, &g)
			checkErrComparison(node, info, &g)
		}
		return true
	})

	return g
}

// checkNilComparison обнаруживает паттерны x == nil / nil == x / x != nil / nil != x,
// где x — имя параметра функции.
func checkNilComparison(b *ast.BinaryExpr, paramNames map[string]bool, info *types.Info, g *model.Guards) {
	lNil := isNilIdent(b.X)
	rNil := isNilIdent(b.Y)

	var candidate ast.Expr
	switch {
	case rNil && !lNil:
		candidate = b.X
	case lNil && !rNil:
		candidate = b.Y
	default:
		return
	}

	if id, ok := candidate.(*ast.Ident); ok && paramNames[id.Name] {
		g.NilCheckedParams[id.Name] = true
	}
}

// checkEmptyStringComparison обнаруживает x == "" / "" == x.
func checkEmptyStringComparison(b *ast.BinaryExpr, paramNames map[string]bool, g *model.Guards) {
	lEmpty := isEmptyStringLit(b.X)
	rEmpty := isEmptyStringLit(b.Y)

	var candidate ast.Expr
	switch {
	case rEmpty && !lEmpty:
		candidate = b.X
	case lEmpty && !rEmpty:
		candidate = b.Y
	default:
		return
	}

	if id, ok := candidate.(*ast.Ident); ok && paramNames[id.Name] {
		g.EmptyCheckedParams[id.Name] = true
	}
}

// checkLenComparison обнаруживает len(x) == 0 / 0 == len(x).
func checkLenComparison(b *ast.BinaryExpr, paramNames map[string]bool, g *model.Guards) {
	var lenArg ast.Expr

	if isZeroLit(b.Y) {
		if call, ok := b.X.(*ast.CallExpr); ok && isLenCall(call) {
			lenArg = call.Args[0]
		}
	} else if isZeroLit(b.X) {
		if call, ok := b.Y.(*ast.CallExpr); ok && isLenCall(call) {
			lenArg = call.Args[0]
		}
	}

	if lenArg == nil {
		return
	}
	if id, ok := lenArg.(*ast.Ident); ok && paramNames[id.Name] {
		g.EmptyCheckedParams[id.Name] = true
	}
}

// checkErrComparison обнаруживает err != nil / err == nil,
// где err — идентификатор типа error (локальная переменная или параметр).
func checkErrComparison(b *ast.BinaryExpr, info *types.Info, g *model.Guards) {
	if isNilIdent(b.Y) {
		if isErrorIdent(b.X, info) {
			g.ErrChecked = true
		}
	} else if isNilIdent(b.X) {
		if isErrorIdent(b.Y, info) {
			g.ErrChecked = true
		}
	}
}

// ── Вспомогательные предикаты ─────────────────────────────────────────────────

func isNilIdent(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "nil"
}

func isEmptyStringLit(e ast.Expr) bool {
	lit, ok := e.(*ast.BasicLit)
	return ok && lit.Value == `""`
}

func isZeroLit(e ast.Expr) bool {
	lit, ok := e.(*ast.BasicLit)
	return ok && lit.Value == "0"
}

func isLenCall(call *ast.CallExpr) bool {
	if len(call.Args) != 1 {
		return false
	}
	fn, ok := call.Fun.(*ast.Ident)
	return ok && fn.Name == "len"
}

// isErrorIdent проверяет, имеет ли идентификатор тип error.
// Использует TypesInfo для точной проверки через type-checker.
func isErrorIdent(e ast.Expr, info *types.Info) bool {
	ident, ok := e.(*ast.Ident)
	if !ok {
		return false
	}
	var obj types.Object
	if o, found := info.Uses[ident]; found {
		obj = o
	} else if o, found := info.Defs[ident]; found {
		obj = o
	}
	if obj == nil {
		return false
	}
	// types.Type.String() для встроенного error возвращает "error".
	return obj.Type().String() == "error"
}

// ── Классификация типов через go/types ───────────────────────────────────────

// classifyExpr определяет TypeKind выражения, используя TypesInfo.
// Если type-checker не предоставил информацию, откатываемся на строковую классификацию.
func classifyExpr(expr ast.Expr, info *types.Info) model.TypeKind {
	if info == nil {
		return classifyStr(exprStr(expr))
	}
	t := info.TypeOf(expr)
	if t == nil {
		return classifyStr(exprStr(expr))
	}
	return classifyType(t)
}

// classifyType преобразует types.Type в TypeKind.
// Проверка time.Time / time.Duration выполняется до switch по Underlying(),
// чтобы struct-обёртки не проваливались в KindStruct.
func classifyType(t types.Type) model.TypeKind {
	if t == nil {
		return model.KindUnknown
	}

	// Сначала проверяем named-типы, требующие специальной обработки.
	if named, ok := t.(*types.Named); ok {
		obj := named.Obj()
		if obj.Pkg() != nil {
			switch obj.Pkg().Path() {
			case "time":
				switch obj.Name() {
				case "Time":
					return model.KindTime
				case "Duration":
					return model.KindDuration
				}
			}
		}
	}

	// Разворачиваем тип до базового.
	switch u := t.Underlying().(type) {
	case *types.Basic:
		switch u.Kind() {
		case types.String:
			return model.KindString
		case types.Bool:
			return model.KindBool
		case types.UntypedNil:
			return model.KindInterface
		case types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
			types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64,
			types.Float32, types.Float64, types.Uintptr:
			return model.KindInt
		}
	case *types.Slice:
		return model.KindSlice
	case *types.Map:
		return model.KindMap
	case *types.Pointer:
		return model.KindPtr
	case *types.Signature:
		return model.KindFunc
	case *types.Chan:
		// Каналы трактуем как непрозрачный тип (nil-фикстура).
		return model.KindInterface
	case *types.Interface:
		// Проверяем встроенный интерфейс error.
		if t.String() == "error" {
			return model.KindError
		}
		return model.KindInterface
	case *types.Struct:
		return model.KindStruct
	}
	return model.KindUnknown
}

// classifyStr — резервная строковая классификация при отсутствии TypesInfo.
// Применяется только если go/packages не смог предоставить типовую информацию.
func classifyStr(s string) model.TypeKind {
	switch s {
	case "string":
		return model.KindString
	case "bool":
		return model.KindBool
	case "error":
		return model.KindError
	case "time.Time":
		return model.KindTime
	case "time.Duration":
		return model.KindDuration
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64", "byte", "rune", "uintptr":
		return model.KindInt
	case "interface{}", "any":
		return model.KindInterface
	}
	switch {
	case strings.HasPrefix(s, "[]"):
		return model.KindSlice
	case strings.HasPrefix(s, "map["):
		return model.KindMap
	case strings.HasPrefix(s, "*"):
		return model.KindPtr
	case strings.HasPrefix(s, "func("):
		return model.KindFunc
	case strings.HasPrefix(s, "chan "):
		return model.KindInterface
	case s != "":
		return model.KindStruct
	}
	return model.KindUnknown
}

// ── Реконструкция строкового представления типа из AST ────────────────────────

// exprStr преобразует AST-выражение типа в строку для кодогенерации.
// Не зависит от type-checker — используется только для TypeStr (вывод в код).
func exprStr(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return exprStr(e.X) + "." + e.Sel.Name
	case *ast.StarExpr:
		return "*" + exprStr(e.X)
	case *ast.ArrayType:
		if e.Len == nil {
			return "[]" + exprStr(e.Elt)
		}
		return "[" + exprStr(e.Len) + "]" + exprStr(e.Elt)
	case *ast.MapType:
		return "map[" + exprStr(e.Key) + "]" + exprStr(e.Value)
	case *ast.InterfaceType:
		// Любой интерфейс упрощаем до interface{} для совместимости с фикстурами.
		return "interface{}"
	case *ast.Ellipsis:
		// Вариадический параметр — представляем как срез для фикстур.
		return "[]" + exprStr(e.Elt)
	case *ast.FuncType:
		// Реконструируем валидную сигнатуру func-типа.
		return funcTypeStr(e)
	case *ast.ChanType:
		return "chan " + exprStr(e.Value)
	case *ast.BasicLit:
		return e.Value // числовой литерал длины массива, например "4"
	case *ast.ParenExpr:
		return exprStr(e.X)
	default:
		return "interface{}"
	}
}

// funcTypeStr реконструирует строку типа func(A, B) C из *ast.FuncType.
// Пример: func(string) bool, func(int, int) (int, error)
func funcTypeStr(ft *ast.FuncType) string {
	var params []string
	if ft.Params != nil {
		for _, f := range ft.Params.List {
			ts := exprStr(f.Type)
			if len(f.Names) == 0 {
				params = append(params, ts)
			} else {
				for range f.Names {
					params = append(params, ts)
				}
			}
		}
	}

	var results []string
	if ft.Results != nil {
		for _, f := range ft.Results.List {
			ts := exprStr(f.Type)
			if len(f.Names) == 0 {
				results = append(results, ts)
			} else {
				for range f.Names {
					results = append(results, ts)
				}
			}
		}
	}

	s := "func(" + strings.Join(params, ", ") + ")"
	switch len(results) {
	case 0:
		// нет возвращаемых значений
	case 1:
		s += " " + results[0]
	default:
		s += " (" + strings.Join(results, ", ") + ")"
	}
	return s
}

// ── Извлечение полей receiver-структуры ──────────────────────────────────────

// receiverFields возвращает список полей receiver-структуры с информацией о том,
// является ли каждое поле интерфейсом. Используется mockplan для поиска
// интерфейс-зависимостей, которые нужно подменить моками.
//
// Алгоритм:
//  1. Резолвим тип receiver через TypesInfo.
//  2. Если это указатель на структуру — берём элемент.
//  3. Если в итоге получили *types.Struct — обходим поля.
//  4. Для каждого поля проверяем types.Type на принадлежность к интерфейсу.
//
// Если тип не структура (например, методы на типе-алиасе) — возвращаем nil.
func receiverFields(pkg *packages.Package, recvType ast.Expr) []model.ReceiverField {
	if pkg == nil || pkg.TypesInfo == nil {
		return nil
	}

	// Резолвим тип receiver. recvType может быть *ast.Ident (T) или *ast.StarExpr (*T).
	tv, ok := pkg.TypesInfo.Types[recvType]
	if !ok || tv.Type == nil {
		return nil
	}

	t := tv.Type
	// Снимаем указатель, если есть.
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}

	// Получаем underlying — структура или нет.
	named, ok := t.(*types.Named)
	if !ok {
		return nil
	}
	st, ok := named.Underlying().(*types.Struct)
	if !ok {
		return nil
	}

	out := make([]model.ReceiverField, 0, st.NumFields())
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		ft := f.Type()

		// Проверяем, является ли тип поля интерфейсом.
		// Important: смотрим на underlying — *types.Interface может быть прямым
		// типом или underlying у *types.Named (для именованных интерфейсов).
		_, isIface := ft.Underlying().(*types.Interface)

		out = append(out, model.ReceiverField{
			Name:        f.Name(),
			TypeStr:     typeStr(ft),
			IsInterface: isIface,
		})
	}
	return out
}

// typeStr — строковое представление types.Type для кодогенерации.
// Использует короткую квалификацию (имя пакета без пути).
func typeStr(t types.Type) string {
	return types.TypeString(t, func(p *types.Package) string {
		if p == nil {
			return ""
		}
		return p.Name()
	})
}

// qualifiedTypeStr возвращает полностью квалифицированное представление типа
// через type-checker. Используется для external test package (_test),
// где типы из исходного пакета должны быть квалифицированы (service.UserRepository).
// При недоступности TypesInfo возвращает exprStr (fallback).
func qualifiedTypeStr(info *types.Info, expr ast.Expr) string {
	if info == nil {
		return exprStr(expr)
	}
	if tv, ok := info.Types[expr]; ok && tv.Type != nil {
		return types.TypeString(tv.Type, func(p *types.Package) string {
			if p == nil {
				return ""
			}
			return p.Name()
		})
	}
	return exprStr(expr)
}

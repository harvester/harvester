package controllergen

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"k8s.io/gengo/args"
	"k8s.io/gengo/v2"
	"k8s.io/gengo/v2/generator"
	"k8s.io/gengo/v2/types"

	cgargs "github.com/rancher/wrangler/v3/pkg/controller-gen/args"
	"github.com/rancher/wrangler/v3/pkg/controller-gen/generators"
	"github.com/sirupsen/logrus"
	"golang.org/x/tools/imports"
	"k8s.io/apimachinery/pkg/runtime/schema"
	csargs "k8s.io/code-generator/cmd/client-gen/args"

	cs "k8s.io/code-generator/cmd/client-gen/generators"
	types2 "k8s.io/code-generator/cmd/client-gen/types"
	dpargs "k8s.io/code-generator/cmd/deepcopy-gen/args"
	dp "k8s.io/code-generator/cmd/deepcopy-gen/generators"
	infargs "k8s.io/code-generator/cmd/informer-gen/args"
	inf "k8s.io/code-generator/cmd/informer-gen/generators"
	lsargs "k8s.io/code-generator/cmd/lister-gen/args"
	ls "k8s.io/code-generator/cmd/lister-gen/generators"
	oaargs "k8s.io/kube-openapi/cmd/openapi-gen/args"
	oa "k8s.io/kube-openapi/pkg/generators"
)

func Run(opts cgargs.Options) {
	genericArgs := args.Default().WithoutDefaultFlagParsing()
	genericArgs.GoHeaderFilePath = opts.Boilerplate
	if genericArgs.OutputBase == "./" { //go modules
		tempDir, err := os.MkdirTemp("", "")
		if err != nil {
			return
		}

		genericArgs.OutputBase = tempDir
		defer os.RemoveAll(tempDir)
	}

	boilerplate, err := genericArgs.LoadGoBoilerplate()
	if err != nil {
		logrus.Fatalf("Loading boilerplate: %v", err)
	}

	customArgs := &cgargs.CustomArgs{
		ImportPackage:      opts.ImportPackage,
		Options:            opts,
		TypesByGroup:       map[schema.GroupVersion][]*types.Name{},
		Package:            opts.OutputPackage,
		OutputBase:         genericArgs.OutputBase,
		BoilerplateContent: boilerplate,
	}
	inputDirs := parseTypes(customArgs)

	clientGen := generators.NewClientGenerator()

	getTargets := func(context *generator.Context) []generator.Target {
		// replace the default formatter options to ensure unused imports are pruned.
		// ref: https://github.com/kubernetes/gengo/pull/277#issuecomment-2557462569
		goGenerator := generator.NewGoFile()
		goGenerator.Format = func(src []byte) ([]byte, error) {
			return imports.Process("", src, nil)
		}
		context.FileTypes[generator.GoFileType] = goGenerator
		return clientGen.GetTargets(context, customArgs)
	}
	if err := gengo.Execute(
		cs.NameSystems(nil),
		cs.DefaultNameSystem(),
		getTargets,
		gengo.StdBuildTag,
		inputDirs,
	); err != nil {
		logrus.Fatalf("Error: %v", err)
	}

	groups := map[string]bool{}
	listerGroups := map[string]bool{}
	informerGroups := map[string]bool{}
	deepCopygroups := map[string]bool{}
	openAPIGroups := map[string]bool{}
	for groupName, group := range customArgs.Options.Groups {
		if group.GenerateTypes {
			deepCopygroups[groupName] = true
		}
		if group.GenerateClients {
			groups[groupName] = true
		}
		if group.GenerateListers {
			listerGroups[groupName] = true
		}
		if group.GenerateInformers {
			informerGroups[groupName] = true
		}
		if group.GenerateOpenAPI {
			openAPIGroups[groupName] = true
		}
	}

	if len(deepCopygroups) == 0 && len(groups) == 0 && len(listerGroups) == 0 && len(informerGroups) == 0 && len(openAPIGroups) == 0 {
		if err := copyGoPathToModules(customArgs); err != nil {
			logrus.Fatalf("go modules copy failed: %v", err)
		}
		return
	}

	if err := copyGoPathToModules(customArgs); err != nil {
		logrus.Fatalf("go modules copy failed: %v", err)
	}

	if err := generateDeepcopy(deepCopygroups, customArgs); err != nil {
		logrus.Fatalf("deepcopy failed: %v", err)
	}

	if err := generateClientset(groups, customArgs); err != nil {
		logrus.Fatalf("clientset failed: %v", err)
	}

	if err := generateListers(listerGroups, customArgs); err != nil {
		logrus.Fatalf("listers failed: %v", err)
	}

	if err := generateInformers(informerGroups, customArgs); err != nil {
		logrus.Fatalf("informers failed: %v", err)
	}

	if err := generateOpenAPI(openAPIGroups, customArgs); err != nil {
		logrus.Fatalf("openapi failed: %v", err)
	}

	if err := copyGoPathToModules(customArgs); err != nil {
		logrus.Fatalf("go modules copy failed: %v", err)
	}
}

func sourcePackagePath(customArgs *cgargs.CustomArgs, pkgName string) string {
	pkgSplit := strings.Split(pkgName, string(os.PathSeparator))
	pkg := filepath.Join(customArgs.OutputBase, strings.Join(pkgSplit[:3], string(os.PathSeparator)))
	return pkg
}

// until k8s code-gen supports gopath
func copyGoPathToModules(customArgs *cgargs.CustomArgs) error {

	pathsToCopy := map[string]bool{}
	for _, types := range customArgs.TypesByGroup {
		for _, names := range types {
			pkg := sourcePackagePath(customArgs, names.Package)
			pathsToCopy[pkg] = true
		}
	}

	pkg := sourcePackagePath(customArgs, customArgs.Package)
	pathsToCopy[pkg] = true

	for pkg := range pathsToCopy {
		if _, err := os.Stat(pkg); os.IsNotExist(err) {
			continue
		}

		return filepath.Walk(pkg, func(path string, info os.FileInfo, err error) error {
			newPath := strings.Replace(path, pkg, ".", 1)
			if info.IsDir() {
				return os.MkdirAll(newPath, info.Mode())
			}

			return copyFile(path, newPath)
		})
	}

	return nil
}

func copyFile(src, dst string) error {
	var err error
	var srcfd *os.File
	var dstfd *os.File
	var srcinfo os.FileInfo

	if srcfd, err = os.Open(src); err != nil {
		return err
	}
	defer srcfd.Close()

	if dstfd, err = os.Create(dst); err != nil {
		return err
	}
	defer dstfd.Close()

	if _, err = io.Copy(dstfd, srcfd); err != nil {
		return err
	}
	if srcinfo, err = os.Stat(src); err != nil {
		return err
	}
	return os.Chmod(dst, srcinfo.Mode())
}

func generateDeepcopy(groups map[string]bool, customArgs *cgargs.CustomArgs) error {
	if len(groups) == 0 {
		return nil
	}

	deepCopyArgs := dpargs.New()
	deepCopyArgs.OutputFile = "zz_generated_deepcopy.go"
	deepCopyArgs.GoHeaderFile = customArgs.Options.Boilerplate

	inputDirs := []string{}
	for gv, names := range customArgs.TypesByGroup {
		if !groups[gv.Group] {
			continue
		}
		inputDirs = append(inputDirs, names[0].Package)
		deepCopyArgs.BoundingDirs = append(deepCopyArgs.BoundingDirs, names[0].Package)
	}

	getTargets := func(context *generator.Context) []generator.Target {
		return dp.GetTargets(context, deepCopyArgs)
	}

	return gengo.Execute(
		dp.NameSystems(),
		dp.DefaultNameSystem(),
		getTargets,
		gengo.StdBuildTag,
		inputDirs,
	)
}

func generateClientset(groups map[string]bool, customArgs *cgargs.CustomArgs) error {
	if len(groups) == 0 {
		return nil
	}

	clientSetArgs := csargs.New()
	clientSetArgs.ClientsetName = "versioned"
	clientSetArgs.OutputDir = filepath.Join(customArgs.OutputBase, customArgs.Package, "clientset")
	clientSetArgs.OutputPkg = filepath.Join(customArgs.Package, "clientset")
	clientSetArgs.GoHeaderFile = customArgs.Options.Boilerplate

	var order []schema.GroupVersion

	for gv := range customArgs.TypesByGroup {
		if !groups[gv.Group] {
			continue
		}
		order = append(order, gv)
	}
	sort.Slice(order, func(i, j int) bool {
		return order[i].Group < order[j].Group
	})

	inputDirs := []string{}
	for _, gv := range order {
		packageName := customArgs.Options.Groups[gv.Group].PackageName
		if packageName == "" {
			packageName = gv.Group
		}
		names := customArgs.TypesByGroup[gv]
		inputDirs = append(inputDirs, names[0].Package)
		clientSetArgs.Groups = append(clientSetArgs.Groups, types2.GroupVersions{
			PackageName: packageName,
			Group:       types2.Group(gv.Group),
			Versions: []types2.PackageVersion{
				{
					Version: types2.Version(gv.Version),
					Package: names[0].Package,
				},
			},
		})
	}
	getTargets := setGenClient(groups, customArgs.TypesByGroup, func(context *generator.Context) []generator.Target {
		return cs.GetTargets(context, clientSetArgs)
	})
	return gengo.Execute(
		cs.NameSystems(nil),
		cs.DefaultNameSystem(),
		getTargets,
		gengo.StdBuildTag,
		inputDirs,
	)
}

func generateOpenAPI(groups map[string]bool, customArgs *cgargs.CustomArgs) error {
	if len(groups) == 0 {
		return nil
	}

	openAPIArgs := oaargs.New()
	openAPIArgs.OutputDir = filepath.Join(customArgs.OutputBase, customArgs.Options.OutputPackage, "openapi")
	openAPIArgs.OutputFile = "zz_generated_openapi.go"
	openAPIArgs.OutputPkg = customArgs.Options.OutputPackage + "/openapi"
	openAPIArgs.GoHeaderFile = customArgs.Options.Boilerplate

	if err := openAPIArgs.Validate(); err != nil {
		return err
	}

	inputDirsMap := map[string]bool{}
	inputDirs := []string{}
	for gv, names := range customArgs.TypesByGroup {
		if !groups[gv.Group] {
			continue
		}

		if _, found := inputDirsMap[names[0].Package]; !found {
			inputDirsMap[names[0].Package] = true
			inputDirs = append(inputDirs, names[0].Package)
		}

		group := customArgs.Options.Groups[gv.Group]
		for _, dep := range group.OpenAPIDependencies {
			if _, found := inputDirsMap[dep]; !found {
				inputDirsMap[dep] = true
				inputDirs = append(inputDirs, dep)
			}
		}
	}

	getTargets := func(context *generator.Context) []generator.Target {
		return oa.GetTargets(context, openAPIArgs)
	}

	return gengo.Execute(
		oa.NameSystems(),
		oa.DefaultNameSystem(),
		getTargets,
		gengo.StdBuildTag,
		inputDirs,
	)
}

func setGenClient(
	groups map[string]bool,
	typesByGroup map[schema.GroupVersion][]*types.Name,
	f func(*generator.Context) []generator.Target,
) func(*generator.Context) []generator.Target {
	return func(context *generator.Context) []generator.Target {
		for gv, names := range typesByGroup {
			if !groups[gv.Group] {
				continue
			}
			for _, name := range names {
				var (
					p           = context.Universe.Package(name.Package)
					t           = p.Type(name.Name)
					status      bool
					nsed        bool
					kubebuilder bool
				)

				for _, line := range append(t.SecondClosestCommentLines, t.CommentLines...) {
					switch {
					case strings.Contains(line, "+kubebuilder:object:root=true"):
						kubebuilder = true
						t.SecondClosestCommentLines = append(t.SecondClosestCommentLines, "+genclient")
					case strings.Contains(line, "+kubebuilder:subresource:status"):
						status = true
					case strings.Contains(line, "+kubebuilder:resource:") && strings.Contains(line, "scope=Namespaced"):
						nsed = true
					}
				}

				if kubebuilder {
					if !nsed {
						t.SecondClosestCommentLines = append(t.SecondClosestCommentLines, "+genclient:nonNamespaced")
					}
					if !status {
						t.SecondClosestCommentLines = append(t.SecondClosestCommentLines, "+genclient:noStatus")
					}

					foundGroup := false
					for _, comment := range p.DocComments {
						if strings.Contains(comment, "+groupName=") {
							foundGroup = true
							break
						}
					}

					if !foundGroup {
						p.DocComments = append(p.DocComments, "+groupName="+gv.Group)
						p.Comments = append(p.Comments, "+groupName="+gv.Group)
						fmt.Println(gv.Group, p.DocComments, p.Comments, p.Path)
					}
				}
			}
		}
		return f(context)
	}
}

func generateInformers(groups map[string]bool, customArgs *cgargs.CustomArgs) error {
	if len(groups) == 0 {
		return nil
	}

	informerArgs := infargs.New()
	informerArgs.VersionedClientSetPackage = filepath.Join(customArgs.Package, "clientset/versioned")
	informerArgs.ListersPackage = filepath.Join(customArgs.Package, "listers")
	informerArgs.OutputDir = filepath.Join(customArgs.OutputBase, customArgs.Package, "informers")
	informerArgs.OutputPkg = filepath.Join(customArgs.Package, "informers")
	informerArgs.GoHeaderFile = customArgs.Options.Boilerplate

	inputDirs := []string{}
	for gv, names := range customArgs.TypesByGroup {
		if !groups[gv.Group] {
			continue
		}
		inputDirs = append(inputDirs, names[0].Package)
	}

	getTargets := setGenClient(groups, customArgs.TypesByGroup, func(context *generator.Context) []generator.Target {
		return inf.GetTargets(context, informerArgs)
	})

	return gengo.Execute(
		inf.NameSystems(nil),
		inf.DefaultNameSystem(),
		getTargets,
		gengo.StdBuildTag,
		inputDirs,
	)
}

func generateListers(groups map[string]bool, customArgs *cgargs.CustomArgs) error {
	if len(groups) == 0 {
		return nil
	}

	listerArgs := lsargs.New()
	listerArgs.OutputDir = filepath.Join(customArgs.OutputBase, customArgs.Package, "listers")
	listerArgs.OutputPkg = filepath.Join(customArgs.Package, "listers")
	listerArgs.GoHeaderFile = customArgs.Options.Boilerplate

	inputDirs := []string{}
	for gv, names := range customArgs.TypesByGroup {
		if !groups[gv.Group] {
			continue
		}
		inputDirs = append(inputDirs, names[0].Package)
	}

	getTargets := setGenClient(groups, customArgs.TypesByGroup, func(context *generator.Context) []generator.Target {
		return ls.GetTargets(context, listerArgs)
	})
	return gengo.Execute(
		ls.NameSystems(nil),
		ls.DefaultNameSystem(),
		getTargets,
		gengo.StdBuildTag,
		inputDirs,
	)
}

func parseTypes(customArgs *cgargs.CustomArgs) []string {
	for groupName, group := range customArgs.Options.Groups {
		if group.GenerateTypes || group.GenerateClients {
			customArgs.Options.Groups[groupName] = group
		}
	}

	for groupName, group := range customArgs.Options.Groups {
		if err := cgargs.ObjectsToGroupVersion(groupName, group.Types, customArgs.TypesByGroup); err != nil {
			// sorry, should really handle this better
			panic(err)
		}
	}

	var inputDirs []string
	for _, names := range customArgs.TypesByGroup {
		inputDirs = append(inputDirs, names[0].Package)
	}

	return inputDirs
}

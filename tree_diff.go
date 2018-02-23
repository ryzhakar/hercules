package hercules

import (
	"io"
	"strings"

	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
)

// TreeDiff generates the list of changes for a commit. A change can be either one or two blobs
// under the same path: "before" and "after". If "before" is nil, the change is an addition.
// If "after" is nil, the change is a removal. Otherwise, it is a modification.
// TreeDiff is a PipelineItem.
type TreeDiff struct {
	previousTree *object.Tree
	SkipDirs     []string
}

const (
	// DependencyTreeChanges is the name of the dependency provided by TreeDiff.
	DependencyTreeChanges = "changes"
	// ConfigTreeDiffSkipBlacklist is the name of the configuration option
	// (TreeDiff.Configure()) which allows to skip blacklist directories.
	ConfigTreeDiffSkipBlacklist = "TreeDiff.SkipVendor"
	// ConfigTreeDiffBlacklistedDirs s the name of the configuration option
	// (TreeDiff.Configure()) which allows to set blacklist directories.
	ConfigTreeDiffBlacklistedDirs = "TreeDiff.BlacklistedDirs"
)

var defaultBlacklistedDirs = []string{"vendor/", "vendors/", "node_modules/"}

// Name of this PipelineItem. Uniquely identifies the type, used for mapping keys, etc.
func (treediff *TreeDiff) Name() string {
	return "TreeDiff"
}

// Provides returns the list of names of entities which are produced by this PipelineItem.
// Each produced entity will be inserted into `deps` of dependent Consume()-s according
// to this list. Also used by hercules.Registry to build the global map of providers.
func (treediff *TreeDiff) Provides() []string {
	arr := [...]string{DependencyTreeChanges}
	return arr[:]
}

// Requires returns the list of names of entities which are needed by this PipelineItem.
// Each requested entity will be inserted into `deps` of Consume(). In turn, those
// entities are Provides() upstream.
func (treediff *TreeDiff) Requires() []string {
	return []string{}
}

// ListConfigurationOptions returns the list of changeable public properties of this PipelineItem.
func (treediff *TreeDiff) ListConfigurationOptions() []ConfigurationOption {
	options := [...]ConfigurationOption{{
		Name:        ConfigTreeDiffSkipBlacklist,
		Description: "Skip blacklist directories.",
		Flag:        "skip-blacklist",
		Type:        BoolConfigurationOption,
		Default:     false}, {
		Name:        ConfigTreeDiffBlacklistedDirs,
		Description: "List of blacklist directories. Separated by comma \",\".",
		Flag:        "blacklisted-dirs",
		Type:        StringsConfigurationOption,
		Default:     defaultBlacklistedDirs},
	}
	return options[:]
}

// Configure sets the properties previously published by ListConfigurationOptions().
func (treediff *TreeDiff) Configure(facts map[string]interface{}) {
	if val, exists := facts[ConfigTreeDiffSkipBlacklist]; exists && val.(bool) == true {
		treediff.SkipDirs = facts[ConfigTreeDiffBlacklistedDirs].([]string)
	}
}

// Initialize resets the temporary caches and prepares this PipelineItem for a series of Consume()
// calls. The repository which is going to be analysed is supplied as an argument.
func (treediff *TreeDiff) Initialize(repository *git.Repository) {
	treediff.previousTree = nil
}

// Consume runs this PipelineItem on the next commit data.
// `deps` contain all the results from upstream PipelineItem-s as requested by Requires().
// Additionally, "commit" is always present there and represents the analysed *object.Commit.
// This function returns the mapping with analysis results. The keys must be the same as
// in Provides(). If there was an error, nil is returned.
func (treediff *TreeDiff) Consume(deps map[string]interface{}) (map[string]interface{}, error) {
	commit := deps["commit"].(*object.Commit)
	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}
	var diff object.Changes
	if treediff.previousTree != nil {
		diff, err = object.DiffTree(treediff.previousTree, tree)
		if err != nil {
			return nil, err
		}
	} else {
		diff = []*object.Change{}
		err = func() error {
			fileIter := tree.Files()
			defer fileIter.Close()
			for {
				file, err := fileIter.Next()
				if err != nil {
					if err == io.EOF {
						break
					}
					return err
				}
				diff = append(diff, &object.Change{
					To: object.ChangeEntry{Name: file.Name, Tree: tree, TreeEntry: object.TreeEntry{
						Name: file.Name, Mode: file.Mode, Hash: file.Hash}}})
			}
			return nil
		}()
		if err != nil {
			return nil, err
		}
	}
	treediff.previousTree = tree

	if len(treediff.SkipDirs) > 0 {
		// filter without allocation
		filteredDiff := diff[:0]
	OUTER:
		for _, change := range diff {
			for _, dir := range treediff.SkipDirs {
				if strings.HasPrefix(change.To.Name, dir) || strings.HasPrefix(change.From.Name, dir) {
					continue OUTER
				}
			}
			filteredDiff = append(filteredDiff, change)
		}

		diff = filteredDiff
	}
	return map[string]interface{}{DependencyTreeChanges: diff}, nil
}

func init() {
	Registry.Register(&TreeDiff{})
}

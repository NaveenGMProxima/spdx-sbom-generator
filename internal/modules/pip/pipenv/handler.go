// SPDX-License-Identifier: Apache-2.0

package pipenv

import (
	"errors"
	"path/filepath"
	"spdx-sbom-generator/internal/helper"
	"spdx-sbom-generator/internal/models"
	"spdx-sbom-generator/internal/modules/pip/worker"
	"strings"
)

const cmdName = "pipenv"
const manifestFile = "Pipfile"
const manifestLockFile = "Pipfile.lock"
const placeholderPkgName = "{PACKAGE}"

var errDependenciesNotFound = errors.New("Unable to generate SPDX file, no modules or vendors found. Please install them before running spdx-sbom-generator, e.g.: `pipenv install` or `pipenv update`")
var errBuildlingModuleDependencies = errors.New("Error building modules dependencies")
var errNoPipCommand = errors.New("No pipenv command")
var errVersionNotFound = errors.New("Python version not found")
var errFailedToConvertModules = errors.New("Failed to convert modules")

type pipenv struct {
	metadata   models.PluginMetadata
	rootModule *models.Module
	command    *helper.Cmd
	basepath   string
	pkgs       []worker.Packages
	metainfo   map[string]*worker.Metadata
}

// New ...
func New() *pipenv {
	return &pipenv{
		metadata: models.PluginMetadata{
			Name:       "The Python Package Index (PyPI)",
			Slug:       "pip",
			Manifest:   []string{manifestLockFile},
			ModulePath: []string{},
		},
	}
}

// Get Metadata ...
func (m *pipenv) GetMetadata() models.PluginMetadata {
	return m.metadata
}

// Is Valid ...
func (m *pipenv) IsValid(path string) bool {
	for i := range m.metadata.Manifest {
		if helper.Exists(filepath.Join(path, m.metadata.Manifest[i])) {
			return true
		}
	}
	return false
}

// Has Modules Installed ...
func (m *pipenv) HasModulesInstalled(path string) error {
	if _, err := m.buildCmd(ModulesCmd, m.basepath); err != nil {
		return err
	}
	result, err := m.command.Output()
	if err == nil && len(result) > 0 && worker.IsRequirementMeet(false, result) {
		m.pkgs = worker.LoadModules(result)
		return nil
	}
	return errDependenciesNotFound
}

// Get Version ...
func (m *pipenv) GetVersion() (string, error) {
	if _, err := m.buildCmd(VersionCmd, m.basepath); err != nil {
		return "", err
	}
	version, err := m.command.Output()
	if err != nil {
		return "Python", errVersionNotFound
	}
	return version, err
}

// Set Root Module ...
func (m *pipenv) SetRootModule(path string) error {
	m.basepath = path
	return nil
}

// Get Root Module ...
func (m *pipenv) GetRootModule(path string) (*models.Module, error) {
	if m.rootModule == nil {
		module, err := m.fetchRootModule(path)
		if err != nil {
			return nil, err
		}
		m.rootModule = &module
	}
	return m.rootModule, nil
}

// List Used Modules...
func (m *pipenv) ListUsedModules(path string) ([]models.Module, error) {
	var modules []models.Module
	mod, err := m.GetRootModule(path)
	if err == nil {
		modules = append(modules, *mod)
	}
	decoder := worker.NewMetadataDecoder(m.GetPackageDetails)
	nonroot := decoder.ConvertMetadataToModules(false, m.pkgs, &modules)
	m.metainfo = worker.MergeMetadataMap(m.metainfo, nonroot)
	return modules, nil
}

// List Modules With Deps ...
func (m *pipenv) ListModulesWithDeps(path string) ([]models.Module, error) {
	modules, err := m.ListUsedModules(path)
	if err != nil {
		return nil, err
	}
	if err := worker.BuildDependencyGraph(&modules, &m.metainfo); err != nil {
		return nil, err
	}
	return modules, err
}

func (m *pipenv) buildCmd(cmd command, path string) (*helper.Cmd, error) {
	cmdArgs := cmd.Parse()
	if cmdArgs[0] != cmdName {
		return nil, errNoPipCommand
	}

	command := helper.NewCmd(helper.CmdOptions{
		Name:      cmdArgs[0],
		Args:      cmdArgs[1:],
		Directory: path,
	})

	m.command = command

	return command, command.Build()
}

func (m *pipenv) GetPackageDetails(packageName string) (string, error) {
	metatdataCmd := command(strings.ReplaceAll(string(MetadataCmd), placeholderPkgName, packageName))

	command, err := m.buildCmd(metatdataCmd, m.basepath)
	result, err := command.Output()
	if err != nil {
		return "", err
	}

	return result, nil
}

func (m *pipenv) PushRootModuleToVenv() bool {
	if _, err := m.buildCmd(InstallRootModuleCmd, m.basepath); err != nil {
		return false
	}
	result, err := m.command.Output()
	if err == nil && len(result) > 0 {
		return true
	}
	return false
}

func (m *pipenv) fetchRootModule(path string) (models.Module, error) {
	var pkgs []worker.Packages
	var modules []models.Module
	var rootModuleState bool

	if worker.IsValidRootModule(path) {
		rootModuleState = m.PushRootModuleToVenv()
	}
	if rootModuleState {
		m.buildCmd(RootModuleCmd, m.basepath)
		result, err := m.command.Output()
		if err == nil && len(result) > 0 && worker.IsRequirementMeet(true, result) {
			pkgs = worker.LoadModules(result)
		}
		decoder := worker.NewMetadataDecoder(m.GetPackageDetails)
		m.metainfo = decoder.ConvertMetadataToModules(true, pkgs, &modules)
	}
	return modules[0], nil
}

// --------------------------------------------------------------------------------------------
// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT license.
// --------------------------------------------------------------------------------------------

package main

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"startupscriptgenerator/common"
	"startupscriptgenerator/common/consts"
	"startupscriptgenerator/node/nodeConsts"
	"strings"
)

type NodeStartupScriptGenerator struct {
	SourcePath                      string
	UserStartupCommand              string
	DefaultAppJsFilePath            string
	BindPort                        string
	UsePm2                          bool
	RemoteDebugging                 bool
	RemoteDebuggingBreakBeforeStart bool
	RemoteDebuggingPort             string
	UseLegacyDebugger               bool //used for node versions < 7.7
	SkipNodeModulesExtraction       bool
}

type packageJson struct {
	Main    string
	Scripts *packageJsonScripts `json:"scripts"`
}

type packageJsonScripts struct {
	Start string `json:"start"`
}

const DefaultBindPort = "8080"
const NodeWrapperPath = "/opt/node-wrapper/"
const LocalIp = "0.0.0.0"
const inspectParamVariableName = "ORYX_NODE_INSPECT_PARAM"

func (gen *NodeStartupScriptGenerator) GenerateEntrypointScript() string {
	logger := common.GetLogger("node.scriptgenerator.GenerateEntrypointScript")
	defer logger.Shutdown()

	logger.LogInformation("Generating script for source at '%s'", gen.SourcePath)

	scriptBuilder := strings.Builder{}
	scriptBuilder.WriteString("#!/bin/sh\n")
	scriptBuilder.WriteString("\n# Enter the source directory to make sure the script runs where the user expects\n")
	scriptBuilder.WriteString("cd " + gen.SourcePath + "\n\n")
	scriptBuilder.WriteString("if [ -f ./" + consts.BuildManifestFileName + " ]; then\n")
	scriptBuilder.WriteString("    echo \"Found '" + consts.BuildManifestFileName + "'\"\n")
	scriptBuilder.WriteString("    . ./" + consts.BuildManifestFileName + "\n")
	scriptBuilder.WriteString("fi\n\n")


	// Expose the port so that a custom command can use it if needed.
	common.SetEnvironmentVariableInScript(&scriptBuilder, "PORT", gen.BindPort, DefaultBindPort)

	if !gen.SkipNodeModulesExtraction {
		scriptBuilder.WriteString("echo \"Checking if node_modules was compressed...\"\n")
		scriptBuilder.WriteString("case $compressedNodeModulesFile in \n")
		scriptBuilder.WriteString("    *\".zip\")\n")
		scriptBuilder.WriteString("        echo \"Found zip-based node_modules.\"\n")
		scriptBuilder.WriteString("        extractionCommand=\"unzip -q $compressedNodeModulesFile -d /node_modules\"\n")
		scriptBuilder.WriteString("        ;;\n")
		scriptBuilder.WriteString("    *\".tar.gz\")\n")
		scriptBuilder.WriteString("        echo \"Found tar.gz based node_modules.\"\n")
		scriptBuilder.WriteString("        extractionCommand=\"tar -xzf $compressedNodeModulesFile -C /node_modules\"\n")
		scriptBuilder.WriteString("         ;;\n")
		scriptBuilder.WriteString("esac\n")
		scriptBuilder.WriteString("if [ ! -z \"$extractionCommand\" ]; then\n")
		scriptBuilder.WriteString("    echo \"Removing existing modules directory...\"\n")
		scriptBuilder.WriteString("    rm -fr /node_modules\n")
		scriptBuilder.WriteString("    mkdir -p /node_modules\n")
		scriptBuilder.WriteString("    echo \"Extracting modules...\"\n")
		scriptBuilder.WriteString("    $extractionCommand\n")
		// Some versions of node, in particular Node 4.8 and 6.2 according to our tests, do not find the node_modules
		// folder at the root. To handle these versions, we also add /node_modules to the NODE_PATH directory.
		scriptBuilder.WriteString("    export NODE_PATH=/node_modules:$NODE_PATH\n")
		// NPM adds the current directory's node_modules/.bin folder to PATH before it runs, so commands in
		// "npm start" can files there. Since we move node_modules, we have to add it to the path ourselves.
		scriptBuilder.WriteString("    export PATH=/node_modules/.bin:$PATH\n")
		// To avoid having older versions of packages available, we delete existing node_modules folder.
		// We do so in the background to not block the app's startup.
		scriptBuilder.WriteString("    if [ -d node_modules ]; then\n")
		// We move the directory first to prevent node from start using it
		scriptBuilder.WriteString("        mv -f node_modules _del_node_modules || true\n")
		scriptBuilder.WriteString("        nohup rm -fr _del_node_modules &> /dev/null &\n")
		scriptBuilder.WriteString("    fi\n")
		scriptBuilder.WriteString("fi\n")
		scriptBuilder.WriteString("echo \"Done.\"\n")
	}

	// If user passed a custom startup command, it should take precedence above all other options
	startupCommand := strings.TrimSpace(gen.UserStartupCommand)
	userStartupCommandFullPath := ""
	if startupCommand != "" {
		userStartupCommandFullPath = filepath.Join(gen.SourcePath, startupCommand)
	}
	commandSource := "User"
	// If the startup command provided by the user is an actual file, we
	// explore it further to see if instead of being a script it is a js
	// or package.json file.
	if startupCommand == "" || common.FileExists(userStartupCommandFullPath) {
		logger.LogVerbose("No user-supplied startup command found")

		// deserialize package.json content
		packageJsonObj, packageJsonPath := getPackageJsonObject(gen.SourcePath, userStartupCommandFullPath)

		startupCommand = gen.getPackageJsonStartCommand(packageJsonObj, packageJsonPath)
		commandSource = "PackageJsonStart"

		if startupCommand == "" {
			logger.LogVerbose("scripts.start not found in package.json")
			startupCommand = gen.getPackageJsonMainCommand(packageJsonObj, packageJsonPath)
			commandSource = "PackageJsonMain"
		}

		if startupCommand == "" {
			startupCommand = gen.getProcessJsonCommand(userStartupCommandFullPath)
			commandSource = "ProcessJson"
		}

		if startupCommand == "" {
			startupCommand = gen.getConfigJsCommand(userStartupCommandFullPath)
			commandSource = "ConfigJs"
		}

		if startupCommand == "" {
			startupCommand = gen.getConfigYamlCommand(userStartupCommandFullPath)
			commandSource = "ConfigYaml"
		}

		if startupCommand == "" {
			startupCommand = gen.getUserProvidedJsFileCommand(userStartupCommandFullPath)
			commandSource = "UserJsFilePath"
		}

		if startupCommand == "" && userStartupCommandFullPath != "" {
			isPermissionAdded := common.ParseCommandAndAddExecutionPermission(gen.UserStartupCommand, gen.SourcePath)
			logger.LogInformation("permission added %t", isPermissionAdded)
			startupCommand = common.ExtendPathForCommand(gen.UserStartupCommand, gen.SourcePath)
			commandSource = "UserScript"
		}

		if startupCommand == "" {
			startupCommand = gen.getCandidateFilesStartCommand(gen.SourcePath)
			commandSource = "CandidateFile"
		}

		if startupCommand == "" {
			logger.LogWarning("Resorting to default startup command")
			startupCommand = gen.getDefaultAppStartCommand()
			commandSource = "DefaultApp"
		}

	} else {

		logger.LogInformation("adding execution permission if needed ...")
		isPermissionAdded := common.ParseCommandAndAddExecutionPermission(gen.UserStartupCommand, gen.SourcePath)
		logger.LogInformation("permission added %t", isPermissionAdded)
		logger.LogInformation("User-supplied startup command: '%s'", gen.UserStartupCommand)
		startupCommand = common.ExtendPathForCommand(startupCommand, gen.SourcePath)
	}

	logger.LogInformation("Looking for App-Insights loader injected by Oryx and export to NODE_OPTIONS if needed")
	scriptBuilder.WriteString("if [ -n $injectedAppInsights ]; then\n")
	scriptBuilder.WriteString("    if [ -f ./oryx-appinsightsloader.js ]; then\n")
	var nodeOptions = "'--require ./" + consts.NodeAppInsightsLoaderFileName + " '$NODE_OPTIONS"
	scriptBuilder.WriteString("        export NODE_OPTIONS=" + nodeOptions + "\n")
	scriptBuilder.WriteString("")
	scriptBuilder.WriteString("    fi\n")
	scriptBuilder.WriteString("fi\n")
	scriptBuilder.WriteString(startupCommand + "\n")

	logger.LogProperties("Finalizing script", map[string]string{"commandSource": commandSource})
	
	var runScript = scriptBuilder.String()
	logger.LogInformation("Run script content:\n" + runScript)
	return runScript
}

func getPackageJsonObject(appPath string, userProvidedPath string) (obj *packageJson, filePath string) {
	logger := common.GetLogger("node.scriptgenerator.GenerateEntrypointScript")
	defer logger.Shutdown()

	const packageFileName = "package.json"
	packageJsonPath := ""

	// We prioritize the file the user provided
	if userProvidedPath != "" {
		if strings.HasSuffix(userProvidedPath, packageFileName) {
			logger.LogInformation("Using user-provided path for packageJson: " + userProvidedPath)
			packageJsonPath = userProvidedPath
		}
	} else {
		logger.LogInformation("Use package.json at the root.")
		packageJsonPath = packageFileName
	}
	if packageJsonPath != "" {
		packageJsonPath = filepath.Join(appPath, packageJsonPath)
		if _, err := os.Stat(packageJsonPath); !os.IsNotExist(err) {
			packageJsonBytes, err := ioutil.ReadFile(packageJsonPath)
			if err != nil {
				return nil, ""
			}

			packageJsonObj := new(packageJson)
			err = json.Unmarshal(packageJsonBytes, &packageJsonObj)
			if err == nil {
				return packageJsonObj, packageJsonPath
			}
		}
	}
	return nil, ""
}

// Gets the startup script from package.json if defined. Returns empty string if not found.
func (gen *NodeStartupScriptGenerator) getPackageJsonStartCommand(packageJsonObj *packageJson, packageJsonPath string) string {
	if packageJsonObj != nil && packageJsonObj.Scripts != nil && packageJsonObj.Scripts.Start != "" {
		var commandBuilder strings.Builder
		if gen.isDebugging() {
			// At this point we know debugging is enabled, and node will be called indirectly through npm
			// or yarn. We inject the wrapper in the path so it executes instead of the node binary.
			debugCommand := gen.getNodeWrapperDebugCommand()
			commandBuilder.WriteString(debugCommand)
		}
		packageJsonDir := filepath.Dir(packageJsonPath)
		isPackageJsonAtRoot := filepath.Clean(packageJsonDir) == filepath.Clean(gen.SourcePath)
		yarnLockPath := filepath.Join(gen.SourcePath, "yarn.lock") // TODO: consolidate with Microsoft.Oryx.BuildScriptGenerator.Node.NodeConstants.YarnLockFileName
		if common.FileExists(yarnLockPath) {
			if !isPackageJsonAtRoot {
				commandBuilder.WriteString("yarn --cwd=" + packageJsonDir + " run start\n")
			} else {
				commandBuilder.WriteString("yarn run start\n")
			}
		} else {
			if !isPackageJsonAtRoot {
				commandBuilder.WriteString("npm --prefix=" + packageJsonDir + " start")
			} else {
				commandBuilder.WriteString("npm start")
			}
			if gen.isDebugging() {
				commandBuilder.WriteString(" --scripts-prepend-node-path false")
			}
			commandBuilder.WriteString("\n")
		}
		return commandBuilder.String()
	}
	return ""
}

// Creates the commands that inject the node wrapper, which will intercept calls to 'node' and
// add the debug option.
func (gen *NodeStartupScriptGenerator) getNodeWrapperDebugCommand() string {
	var commandBuilder strings.Builder
	// At this point we know debugging is enabled, and node will be called indirectly through npm
	// or yarn. We inject the wrapper in the path so it executes instead of the node binary.
	commandBuilder.WriteString("export PATH=" + NodeWrapperPath + ":$PATH\n")
	debugFlag := gen.getDebugFlag()
	commandBuilder.WriteString("export " + inspectParamVariableName + "=\"" + debugFlag + "\"\n")
	return commandBuilder.String()
}

// Gets the startup script from package.json if defined. Returns empty string if not found.
func (gen *NodeStartupScriptGenerator) getPackageJsonMainCommand(packageJsonObj *packageJson, packageJsonPath string) string {
	if packageJsonObj != nil && packageJsonObj.Main != "" {
		logger := common.GetLogger("node.scriptgenerator.getPackageJsonMainCommand")
		defer logger.Shutdown()
		logger.LogVerbose("Using startup command from package.json main field")
		startupFilePath := packageJsonObj.Main
		packageJsonDir := filepath.Dir(packageJsonPath)
		// If package.json is not at the root, we need to add the path to it
		// to the script, since we assume the script in the main property
		// has the path relative to package.json.
		if filepath.Clean(packageJsonDir) != filepath.Clean(gen.SourcePath) {
			subPath := common.GetSubPath(gen.SourcePath, packageJsonDir)
			startupFilePath = filepath.Join(subPath, startupFilePath)
		}
		startupCommand := gen.getStartupCommandFromJsFile(startupFilePath)
		return startupCommand
	}
	return ""
}

func (gen *NodeStartupScriptGenerator) getProcessJsonCommand(userInputPath string) string {
	processJsonPath := gen.checkStartupFileWithPath(".json", "process.json", userInputPath)
	if processJsonPath != "" {
		debugCommand := ""
		if gen.isDebugging() {
			debugCommand = gen.getNodeWrapperDebugCommand()
		}
		return debugCommand + getPm2StartCommand(processJsonPath)
	}
	return ""
}

func (gen *NodeStartupScriptGenerator) getConfigJsCommand(userInputFullPath string) string {
	configJsFullPath := gen.checkStartupFileWithPath(".config.js", "ecosystem.config.js", userInputFullPath)
	if configJsFullPath != "" {
		return getPm2StartCommand(configJsFullPath)
	}
	return ""
}

func (gen *NodeStartupScriptGenerator) getConfigYamlCommand(userInputFullPath string) string {
	configYamlFullPath := gen.checkStartupFileWithPath(".yml", "", userInputFullPath)

	if configYamlFullPath == "" {
		configYamlFullPath = gen.checkStartupFileWithPath(".yaml", "", userInputFullPath)
	}
	if configYamlFullPath != "" {
		return getPm2StartCommand(userInputFullPath)
	}
	return ""
}

func (gen *NodeStartupScriptGenerator) getUserProvidedJsFileCommand(fileFullPath string) string {
	command := ""
	if strings.HasSuffix(fileFullPath, ".js") {
		subPath := common.GetSubPath(gen.SourcePath, fileFullPath)
		command = gen.getStartupCommandFromJsFile(subPath)
	}
	return command
}

// Try to find the main file for the app
func (gen *NodeStartupScriptGenerator) getCandidateFilesStartCommand(appPath string) string {
	logger := common.GetLogger("node.scriptgenerator.getCandidateFilesStartCommand")
	defer logger.Shutdown()

	startupFileCommand := ""
	filesToSearch := []string{"bin/www", "server.js", "app.js", "index.js", "hostingstart.js"}

	for _, file := range filesToSearch {
		fullPath := filepath.Join(gen.SourcePath, file)
		if common.FileExists(fullPath) {
			logger.LogInformation("Found startup candidate '%s'", fullPath)
			startupFileCommand = gen.getStartupCommandFromJsFile(file)
			break
		}
	}

	return startupFileCommand
}

func (gen *NodeStartupScriptGenerator) getDefaultAppStartCommand() string {
	startupCommand := ""
	if gen.DefaultAppJsFilePath != "" {
		startupCommand = gen.getStartupCommandFromJsFile(gen.DefaultAppJsFilePath)
	}
	return startupCommand
}

func (gen *NodeStartupScriptGenerator) isDebugging() bool {
	return gen.RemoteDebugging || gen.RemoteDebuggingBreakBeforeStart
}

// Look for a configuration file called 'configFileName' in the user repo, or if the user provided a path
// to a file as the startup command, checks if the path refers to a config file by using 'configFileSuffix'.
// If the config file exists on disk, its full path is returned. Otherwise it returns an empty string.
func (gen *NodeStartupScriptGenerator) checkStartupFileWithPath(configFileSuffix string, configFileName string, userInputFullPath string) string {
	configFilePath := ""
	if userInputFullPath != "" {
		if strings.HasSuffix(userInputFullPath, configFileSuffix) {
			configFilePath = userInputFullPath
		}
	} else if configFileName != "" {
		configFilePath = filepath.Join(gen.SourcePath, configFileName)
	}

	if configFilePath != "" && common.FileExists(configFilePath) {
		return configFilePath
	}
	return ""

}

func (gen *NodeStartupScriptGenerator) getStartupCommandFromJsFile(mainJsFilePath string) string {
	logger := common.GetLogger("node.scriptgenerator.getStartupCommandFromJsFile")
	defer logger.Shutdown()

	var commandBuilder strings.Builder
	if gen.RemoteDebugging || gen.RemoteDebuggingBreakBeforeStart {
		logger.LogInformation("Remote debugging on")
		debugFlag := gen.getDebugFlag()
		commandBuilder.WriteString("node " + debugFlag)
	} else if gen.UsePm2 {
		commandBuilder.WriteString(getPm2StartCommand(""))
	} else {
		commandBuilder.WriteString("node")
	}

	commandBuilder.WriteString(" " + mainJsFilePath)
	return commandBuilder.String()
}

func getPm2StartCommand(filePath string) string {
	if filePath != "" {
		filePath = " " + filePath
	}
	command := "pm2 start" + filePath + " --no-daemon"
	return command
}

// Builds the flag that should be passed to `node` to enable debugging.
func (gen *NodeStartupScriptGenerator) getDebugFlag() string {
	var commandBuilder strings.Builder
	if gen.UseLegacyDebugger {
		commandBuilder.WriteString("--debug")
	} else {
		commandBuilder.WriteString("--inspect")
	}

	if gen.RemoteDebuggingBreakBeforeStart {
		commandBuilder.WriteString("-brk")
	}

	commandBuilder.WriteString("=" + LocalIp)
	if gen.RemoteDebuggingPort != "" {
		commandBuilder.WriteString(":" + gen.RemoteDebuggingPort)
	}

	return commandBuilder.String()
}

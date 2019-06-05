// --------------------------------------------------------------------------------------------
// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT license.
// --------------------------------------------------------------------------------------------

package main

import (
	"path/filepath"
	"startupscriptgenerator/common"
	"strings"
)

type DotnetCoreStartupScriptGenerator struct {
	AppPath            string
	RunFromPath        string
	UserStartupCommand string
	DefaultAppFilePath string
	BindPort           string
	Manifest           common.BuildManifest
}

const DefaultBindPort = "8080"
const RuntimeConfigJsonExtension = "runtimeconfig.json"

func (gen *DotnetCoreStartupScriptGenerator) GenerateEntrypointScript(scriptBuilder *strings.Builder) string {
	logger := common.GetLogger("dotnetcore.scriptgenerator.GenerateEntrypointScript")
	defer logger.Shutdown()

	logger.LogInformation("Generating script for published output at '%s'", gen.AppPath)

	// Expose the port so that a custom command can use it if needed
	common.SetEnvironmentVariableInScript(scriptBuilder, "PORT", gen.BindPort, DefaultBindPort)
	scriptBuilder.WriteString("export ASPNETCORE_URLS=http://*:$PORT\n\n")

	defaultAppFileDir := filepath.Dir(gen.DefaultAppFilePath)

	scriptBuilder.WriteString("readonly appPath=\"" + gen.RunFromPath + "\"\n")
	scriptBuilder.WriteString("userStartUpCommand=(" + gen.UserStartupCommand + ")\n")
	scriptBuilder.WriteString("startUpCommand=\"\"\n")
	scriptBuilder.WriteString("readonly defaultAppFileDir=\"" + defaultAppFileDir + "\"\n")
	scriptBuilder.WriteString("readonly defaultAppFilePath=\"" + gen.DefaultAppFilePath + "\"\n")

	script := `
isLinuxExecutable() {
	local file="$1"
	if [ -x "$file" ] && file "$file" | grep -q "GNU/Linux"
	then
	  isLinuxExecutableResult="true"
	else
	  isLinuxExecutableResult="false"
	fi
}

cd "$appPath"
len=${#userStartUpCommand[@]}
if [ $len -ne 0 ]; then
	if [ $len -eq 1 ]; then
		file="${userStartUpCommand[0]}"
		if [ -f "$file" ]; then
			startUpCommand="${userStartUpCommand[@]}"
		else
		  	echo "Could not find the startup file '$file' on disk."
		fi
	elif [ $len -eq 2 ] && [ "${userStartUpCommand[0]}" == "dotnet" ]; then
		if [ -f "${userStartUpCommand[1]}" ]; then
		  	startUpCommand="${userStartUpCommand[@]}"
		  	echo "Using the startup command '$startUpCommand' provided by the user to start the app."
		else
			echo "WARNING: Error in user provided startup command '${userStartUpCommand[@]}'."
			echo "WARNING: Could not find the file '${userStartUpCommand[1]}' on disk."
		fi
	else
		startUpCommand="${userStartUpCommand[@]}"
		echo "Using the startup command '$startUpCommand' provided by the user to start the app."
	fi
fi

if [ -z "$startUpCommand" ]; then
	echo Finding the startup file name...
	runtimeConfigJsonFiles=()
	for file in *; do
		if [ -f "$file" ]; then
			case $file in
				*.runtimeconfig.json)
					runtimeConfigJsonFiles+=("$file")
				;;
			esac
		fi
	done

	fileCount=${#runtimeConfigJsonFiles[@]}
	if [ $fileCount -eq 1 ]; then
		file=${runtimeConfigJsonFiles[0]}
		startupDllFileNamePrefix=${file%%.runtimeconfig.json}
		startupExecutableFileName="$startupDllFileNamePrefix"
		startupDllFileName="$startupDllFileNamePrefix.dll"
	else
		echo
		echo "WARNING: Unable to find the startup dll file name."
		echo "WARNING: Expected to find only one file with extension 'runtimeconfig.json' but found $fileCount"
		echo "WARNING: Found files: ${runtimeConfigJsonFiles[@]}"
		echo "WARNING: To fix this issue you can set the startup command to point to a particular startup file, for example: 'dotnet myapp.dll'"
	fi

	if [ -f "$startupExecutableFileName" ]; then
		# Starting ASP.NET Core 3.0, an executable is created based on the platform where it is published from,
		# so for example, if a user does a publish (not self-contained) on Mac, there would be files like 'todoApp'
		# and 'todoApp.dll'. In this scenario the 'todoApp' executable is actually meant for Mac and not for Linux.
		# So here we check for the file type and fall back to using 'dotnet todoApp.dll'.

		isLinuxExecutable $startupExecutableFileName
		if [ "$isLinuxExecutableResult" == "true" ]; then
			echo "Found the startup executable file '$startupExecutableFileName'"
			startUpCommand="./$startupExecutableFileName"
		else
			echo "Cannot use executable '$startupExecutableFileName' as startup file as it is not meant for Linux"
		fi
	fi

	if [ -z "$startUpCommand" ]; then
		if [ -f "$startupDllFileName" ]; then
			echo "Found the startup file '$startupDllFileName'"
			startUpCommand="dotnet '$startupDllFileName'"
		else
			echo "Cound not find the startup dll file '$startupDllFileName'"
		fi
	fi
fi

if [ -z "$startUpCommand" ]; then
	echo "Could not generate a startup command to start the app. Trying to use the default app instead..."
	if [ -f "$defaultAppFilePath" ]; then
		cd "$defaultAppFileDir"
		startUpCommand="dotnet '$defaultAppFilePath'"
	else
		echo "Could not find the default app file '$defaultAppFilePath'"
		echo Unable to start the application.
		exit 1
	fi
fi

echo "Running the command '$startUpCommand'..."
eval "$startUpCommand"
`
	scriptBuilder.WriteString(script)
	var runScript = scriptBuilder.String()
	logger.LogInformation("Run script content:\n" + runScript)
	return runScript
}

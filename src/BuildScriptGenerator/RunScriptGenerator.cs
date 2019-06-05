﻿// --------------------------------------------------------------------------------------------
// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT license.
// --------------------------------------------------------------------------------------------

using System;
using System.Collections.Generic;
using System.Linq;
using Microsoft.Oryx.BuildScriptGenerator.Exceptions;
using Microsoft.Oryx.Common.Extensions;

namespace Microsoft.Oryx.BuildScriptGenerator
{
    internal class RunScriptGenerator : IRunScriptGenerator
    {
        private readonly IEnumerable<IProgrammingPlatform> _programmingPlatforms;

        public RunScriptGenerator(IEnumerable<IProgrammingPlatform> programmingPlatforms)
        {
            _programmingPlatforms = programmingPlatforms;
        }

        public string GenerateBashScript(string targetPlatformName, RunScriptGeneratorOptions options)
        {
            var targetPlatform = _programmingPlatforms
                .Where(p => p.Name.EqualsIgnoreCase(targetPlatformName))
                .FirstOrDefault();

            if (targetPlatform == null)
            {
                throw new UnsupportedLanguageException($"Platform '{targetPlatformName}' is not supported.");
            }

            var runScript = targetPlatform.GenerateBashRunScript(options);
            return runScript;
        }
    }
}
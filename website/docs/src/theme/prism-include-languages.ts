import siteConfig from '@generated/docusaurus.config';
import type * as PrismNamespace from 'prismjs';
import type { Optional } from 'utility-types';

export default function prismIncludeLanguages(
  PrismObject: typeof PrismNamespace,
): void {
  const {
    themeConfig: {prism},
  } = siteConfig;
  const {additionalLanguages} = prism as {additionalLanguages: string[]};
  const PrismBefore = globalThis.Prism;
  globalThis.Prism = PrismObject;

  additionalLanguages.forEach((lang) => {
    if (lang === 'php') {
      require('prismjs/components/prism-markup-templating.js');
    }
    require(`prismjs/components/prism-${lang}`);
  });

  addMorphCommandLanguage(PrismObject);

  delete (globalThis as Optional<typeof globalThis, 'Prism'>).Prism;
  if (typeof PrismBefore !== 'undefined') {
    globalThis.Prism = PrismObject;
  }
}

function addMorphCommandLanguage(PrismObject: typeof PrismNamespace): void {
  const bash = PrismObject.languages.bash;
  if (!bash) {
    return;
  }

  const morphSubcommands = [
    'approve',
    'auth',
    'clear-pending',
    'compact',
    'config',
    'current',
    'daemon',
    'database',
    'doctor',
    'gateway',
    'get',
    'init',
    'list',
    'login',
    'logout',
    'new',
    'pairing',
    'path',
    'profile',
    'repair',
    'restart',
    'revoke',
    'session',
    'set',
    'start',
    'status',
    'stop',
    'trace',
    'unarchive',
    'use',
    'version',
    'view',
  ].join('|');
  const morphGlobalFlags =
    String.raw`(?:\s+(?:(?:--profile|-p|--config|--env-file)\s+\S+|--trace\.enabled(?:=\S+)?))*`;

  PrismObject.languages.insertBefore('bash', 'function', {
    'morph-command': {
      pattern: /(^|[|&;]\s*|\b[A-Z_][A-Z0-9_]*=\S+\s+)(?:\.\/build\/)?morph(?=\s|$)/m,
      lookbehind: true,
      alias: 'function',
    },
  });
  PrismObject.languages.insertBefore('bash', 'parameter', {
    'morph-subcommand': {
      pattern: new RegExp(
        String.raw`((?:^|[|&;]\s*|\b[A-Z_][A-Z0-9_]*=\S+\s+)(?:\.\/build\/)?morph${morphGlobalFlags}\s+)(?:(?:${morphSubcommands})(?=\s|$)\s*)+`,
        'm',
      ),
      lookbehind: true,
      alias: 'keyword',
    },
  });

  PrismObject.languages.morph = bash;
  PrismObject.languages.console = bash;
}

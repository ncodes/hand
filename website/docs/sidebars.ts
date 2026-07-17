import type {SidebarsConfig} from '@docusaurus/plugin-content-docs';

const sidebars: SidebarsConfig = {
  docs: [
    'index',
    {
      type: 'category',
      label: 'Getting Started',
      collapsed: false,
      items: [
        'getting-started/quickstart',
        'getting-started/installation',
        'getting-started/first-chat',
        'getting-started/profiles-and-config',
        'getting-started/learning-path',
      ],
    },
    {
      type: 'category',
      label: 'Concepts',
      collapsed: false,
      items: [
        'concepts/architecture',
        'concepts/daemon-and-rpc',
        'concepts/profiles',
        'concepts/sessions',
        'concepts/memory',
        'concepts/tools',
        'concepts/gateways',
        'concepts/safety-and-guardrails',
        'concepts/permissions',
        'concepts/automation',
      ],
    },
    {
      type: 'category',
      label: 'Guides',
      collapsed: false,
      items: [
        'guides/tui',
        'guides/provider-auth',
        'guides/local-models',
        'guides/config',
        'guides/sessions',
        'guides/memory',
        'guides/automation',
        'guides/search-and-traces',
        {
          type: 'category',
          label: 'Gateway',
          items: [
            'guides/gateway/index',
            'guides/gateway/generic-http',
            'guides/gateway/telegram',
            'guides/gateway/slack',
            'guides/gateway/pairing-and-allowlists',
          ],
        },
        'guides/troubleshooting',
      ],
    },
    {
      type: 'category',
      label: 'Operations',
      items: [
        'operations/daemon',
        'operations/gateway-management',
        'operations/doctor',
        'operations/security',
        'operations/automation',
        'operations/backups-and-state',
      ],
    },
    {
      type: 'category',
      label: 'Reference',
      items: [
        'reference/cli',
        'reference/slash-commands',
        'reference/config',
        'reference/environment-variables',
        'reference/gateway-routes',
        'reference/automation',
        'reference/rpc',
        'reference/trace-events',
        'reference/faq',
      ],
    },
  ],
};

export default sidebars;

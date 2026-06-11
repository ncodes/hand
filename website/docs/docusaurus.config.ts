import {themes as prismThemes} from 'prism-react-renderer';
import type {Config} from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';

const config: Config = {
  title: 'Hand',
  tagline: 'A terminal-first personal agent',
  favicon: 'img/favicon.ico',

  future: {
    v4: true,
  },

  url: 'https://hand.local',
  baseUrl: '/',

  organizationName: 'wandxy',
  projectName: 'hand',

  onBrokenLinks: 'throw',

  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  presets: [
    [
      'classic',
      {
        docs: {
          sidebarPath: './sidebars.ts',
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
      } satisfies Preset.Options,
    ],
  ],

  themeConfig: {
    image: 'img/docusaurus-social-card.jpg',
    colorMode: {
      defaultMode: 'dark',
      respectPrefersColorScheme: true,
    },
    navbar: {
      title: 'Hand',
      logo: {
        alt: 'Hand',
        src: 'img/logo.svg',
      },
      items: [
        {
          type: 'docSidebar',
          sidebarId: 'docs',
          position: 'left',
          label: 'Guide',
        },
        {
          to: '/docs/guides/gateway',
          label: 'Gateway',
          position: 'left',
        },
        {
          to: '/docs/reference/cli',
          label: 'Reference',
          position: 'left',
        },
        {
          href: 'https://github.com/wandxy/hand',
          label: 'GitHub',
          position: 'right',
        },
      ],
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
    },
  } satisfies Preset.ThemeConfig,
};

export default config;

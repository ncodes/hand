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
      logo: {
        alt: 'Hand',
        src: 'img/logo-black.svg',
        srcDark: 'img/logo-white2.svg',
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
          type: 'dropdown',
          label: 'Community',
          position: 'right',
          items: [
            {
              label: 'Forum',
              href: 'https://github.com/wandxy/hand/discussions',
            },
            {
              label: 'Twitter',
              href: 'https://x.com/wandxy',
            },
            {
              label: 'Discord',
              href: 'https://discord.com/invite/wandxy',
            },
          ],
        },
        {
          type: 'custom-socialIcon',
          className: 'navbar-social-link-start',
          href: 'https://x.com/wandxy',
          icon: 'twitter',
          label: 'Twitter',
          position: 'right',
        },
        {
          type: 'custom-socialIcon',
          href: 'https://discord.com/invite/wandxy',
          icon: 'discord',
          label: 'Discord',
          position: 'right',
        },
        {
          type: 'custom-socialIcon',
          href: 'https://github.com/wandxy/hand',
          icon: 'github',
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

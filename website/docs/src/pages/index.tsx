import type {ReactNode} from 'react';
import clsx from 'clsx';
import Link from '@docusaurus/Link';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import Layout from '@theme/Layout';
import Heading from '@theme/Heading';

import styles from './index.module.css';

function HomepageHeader() {
  const {siteConfig} = useDocusaurusContext();
  return (
    <header className={clsx(styles.heroBanner)}>
      <div className="container">
        <p className={styles.eyebrow}>Hand Docs</p>
        <Heading as="h1" className={styles.title}>
          {siteConfig.title}
        </Heading>
        <p className={styles.subtitle}>
          {siteConfig.tagline} with durable sessions, memory, search, and
          daemon-owned gateways for Slack, Telegram, and HTTP.
        </p>
        <div className={styles.buttons}>
          <Link
            className="button button--secondary button--lg"
            to="/docs/getting-started/quickstart">
            Start with Hand
          </Link>
          <Link
            className="button button--outline button--secondary button--lg"
            to="/docs/guides/gateway">
            Gateway docs
          </Link>
        </div>
      </div>
    </header>
  );
}

export default function Home(): ReactNode {
  const {siteConfig} = useDocusaurusContext();
  return (
    <Layout
      title={siteConfig.title}
      description="Documentation for Hand, a terminal-first personal agent.">
      <HomepageHeader />
      <main>
        <section className={styles.sections}>
          <div className="container">
            <div className="row">
              <div className="col col--4">
                <article className={styles.card}>
                <Heading as="h2">Run Hand</Heading>
                <p>
                  Install, configure a profile, start the daemon, and use the
                  terminal UI for daily agent work.
                </p>
                <Link to="/docs/getting-started/quickstart">Quickstart</Link>
                </article>
              </div>
              <div className="col col--4">
                <article className={styles.card}>
                <Heading as="h2">Use Memory</Heading>
                <p>
                  Understand sessions, search, traces, and durable memory across
                  conversations and surfaces.
                </p>
                <Link to="/docs/concepts/memory">Memory concepts</Link>
                </article>
              </div>
              <div className="col col--4">
                <article className={styles.card}>
                <Heading as="h2">Connect Gateways</Heading>
                <p>
                  Bring the same daemon-backed agent to Slack, Telegram, and
                  generic HTTP integrations.
                </p>
                <Link to="/docs/guides/gateway">Gateway overview</Link>
                </article>
              </div>
            </div>
          </div>
        </section>
      </main>
    </Layout>
  );
}

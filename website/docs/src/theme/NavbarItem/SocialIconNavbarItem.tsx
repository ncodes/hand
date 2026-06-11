import Link from '@docusaurus/Link';
import {DiscordIcon, GithubIcon, NewTwitterIcon} from '@hugeicons/core-free-icons';
import {HugeiconsIcon} from '@hugeicons/react';
import clsx from 'clsx';
import type {ReactNode} from 'react';

const icons = {
  discord: DiscordIcon,
  github: GithubIcon,
  twitter: NewTwitterIcon,
};

type SocialIconName = keyof typeof icons;

type Props = {
  className?: string;
  href: string;
  icon: SocialIconName;
  label: string;
  mobile?: boolean;
};

export default function SocialIconNavbarItem({
  className,
  href,
  icon,
  label,
  mobile,
}: Props): ReactNode {
  if (mobile) {
    return (
      <Link className={clsx('menu__link', className)} href={href}>
        {label}
      </Link>
    );
  }

  return (
    <Link
      aria-label={label}
      className={clsx('navbar__item navbar__link navbar-social-link', className)}
      href={href}
      title={label}
    >
      <HugeiconsIcon
        color="currentColor"
        icon={icons[icon]}
        size={18}
        strokeWidth={1.8}
      />
    </Link>
  );
}

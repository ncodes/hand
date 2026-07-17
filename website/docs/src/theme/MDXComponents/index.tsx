import React, {type ComponentProps} from 'react';
import MDXComponents from '@theme-original/MDXComponents';

function Table(props: ComponentProps<'table'>) {
  return (
    <div className="morph-table-scroll">
      <table {...props} />
    </div>
  );
}

export default {
  ...MDXComponents,
  table: Table,
};

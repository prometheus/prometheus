import React, { FC, useState } from 'react';
import { getColor, Target } from './target';
import { Badge, Button, Modal, ModalBody, ModalHeader, Table } from 'reactstrap';
import TargetLabels from './TargetLabels';
import styles from './ScrapePoolPanel.module.css';
import { formatRelative } from '../../utils';
import { now } from 'moment';
import TargetScrapeDuration from './TargetScrapeDuration';
import EndpointLink from './EndpointLink';
import CustomInfiniteScroll, { InfiniteScrollItemsProps } from '../../components/CustomInfiniteScroll';
import { usePathPrefix } from '../../contexts/PathPrefixContext';
import { useTargetScrapeProxyEnabled } from '../../contexts/TargetScrapeProxyContext';

const baseColumns = ['Endpoint', 'State', 'Labels', 'Last Scrape', 'Scrape Duration', 'Error'];

interface ScrapePoolContentProps {
  targets: Target[];
}

const ScrapePoolContentTable: FC<InfiniteScrollItemsProps<Target>> = ({ items }) => {
  const pathPrefix = usePathPrefix();
  const enabled = useTargetScrapeProxyEnabled();
  const [modalOpen, setModalOpen] = useState(false);
  const [modalTitle, setModalTitle] = useState('');
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [body, setBody] = useState('');

  const openMetrics = async (t: Target) => {
    if (!enabled) {
      return;
    }
    setModalTitle(`${t.scrapePool} — ${t.scrapeUrl}`);
    setErr(null);
    setBody('');
    setLoading(true);
    setModalOpen(true);
    try {
      const params = new URLSearchParams({
        scrapePool: t.scrapePool,
        scrapeUrl: t.scrapeUrl,
      });
      const resp = await fetch(`${pathPrefix}/api/v1/targets/scrape?${params.toString()}`);
      const text = await resp.text();
      if (!resp.ok) {
        throw new Error(text || resp.statusText);
      }
      setBody(text);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };

  const copy = async () => {
    if (!body) return;
    await navigator.clipboard.writeText(body);
  };

  const download = () => {
    const blob = new Blob([body], { type: 'text/plain;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'metrics.txt';
    a.click();
    URL.revokeObjectURL(url);
  };

  const columns = enabled ? [...baseColumns, 'Metrics'] : baseColumns;

  return (
    <>
      <Modal isOpen={modalOpen} toggle={() => setModalOpen(false)} size="lg">
        <ModalHeader toggle={() => setModalOpen(false)}>{modalTitle}</ModalHeader>
        <ModalBody>
          <div className="d-flex justify-content-end mb-2">
            <Button color="secondary" size="sm" className="me-2" disabled={loading || body === ''} onClick={copy}>
              Copy
            </Button>
            <Button color="secondary" size="sm" disabled={loading || body === ''} onClick={download}>
              Download
            </Button>
          </div>
          {err ? <div className="text-danger mb-2">{err}</div> : null}
          <textarea
            className="form-control"
            style={{ fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace' }}
            rows={20}
            readOnly
            value={loading ? 'Loading…' : body}
          />
        </ModalBody>
      </Modal>

      <Table className={styles.table} size="sm" bordered hover striped>
        <thead>
          <tr key="header">
            {columns.map((column) => (
              <th key={column}>{column}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {items.map((target, index) => (
            <tr key={index}>
              <td className={styles.endpoint}>
                <EndpointLink endpoint={target.scrapeUrl} globalUrl={target.globalUrl} />
              </td>
              <td className={styles.state}>
                <Badge color={getColor(target.health)}>{target.health.toUpperCase()}</Badge>
              </td>
              <td className={styles.labels}>
                <TargetLabels discoveredLabels={target.discoveredLabels} labels={target.labels} />
              </td>
              <td className={styles['last-scrape']}>{formatRelative(target.lastScrape, now())}</td>
              <td className={styles['scrape-duration']}>
                <TargetScrapeDuration
                  duration={target.lastScrapeDuration}
                  scrapePool={target.scrapePool}
                  idx={index}
                  interval={target.scrapeInterval}
                  timeout={target.scrapeTimeout}
                />
              </td>
              <td className={styles.errors}>
                {target.lastError ? <span className="text-danger">{target.lastError}</span> : null}
              </td>
              {enabled ? (
                <td>
                  <Button color="secondary" size="sm" onClick={() => openMetrics(target)}>
                    View metrics
                  </Button>
                </td>
              ) : null}
            </tr>
          ))}
        </tbody>
      </Table>
    </>
  );
};

export const ScrapePoolContent: FC<ScrapePoolContentProps> = ({ targets }) => {
  return <CustomInfiniteScroll allItems={targets} child={ScrapePoolContentTable} />;
};

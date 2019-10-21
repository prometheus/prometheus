import React from 'react';
import ReactDOM from 'react-dom';
import { act } from "react-dom/test-utils";
import QueryStats from './QueryStats';

let container = null;
beforeEach(() => {
    container = document.createElement('div');
    document.body.appendChild(container);
});

afterEach(() => {
    ReactDOM.unmountComponentAtNode(container);
    container.remove();
    container = null;
})

it('does not render when query is loading', () => {
    act(() => {
        ReactDOM.render(<QueryStats loading="true" stats={{}} />, container);
    });
    expect(container.children).toBe.undefined;
});

it('does render when query is finished', () => {
    const stats = {
        loadTime: 17,
        resolution: 14,
        totalTimeSeries: 1,
    };
    act(() => {
        ReactDOM.render(<QueryStats loading={false} stats={stats} />, container);
    });
    expect(container.textContent).toBe("Load Time: 17ms   Resolution: 14   Total Time Series: 1");
});

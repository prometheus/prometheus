import React, { Component } from 'react';
import { RouteComponentProps } from '@reach/router';
import PathPrefixProps from '../PathPrefixProps';

// const Services: FC<RouteComponentProps & PathPrefixProps> = ({ pathPrefix }) => {
//   console.warn(`${pathPrefix}/api/v1/targets`)
//   // const { response, error } = useFetch<
//   fetch(`${pathPrefix}/api/v1/targets`, { cache: 'no-store' }).then(resp => {
//     console.warn('checeker')
//     if (resp.ok) {
//       return resp.json();
//     }
//   }).then(res => {
//     console.warn('res is ')
//     console.warn(res.data)
//   })
//   return (
//     <>
//       <h2>Service Discovery</h2>
//       <Alert color="warning">
//         This page is still under construction. Please try it in the <a href={`${pathPrefix}/service-discovery`}>Classic UI</a>.
//       </Alert>
//     </>
//   );
// };

interface ServicesState {
  response: Object;
  fetchServicesErrorMessage: string;
  targets: any;
}

interface targetsComponent {
  name: string;
  total: number;
  active: number;
}

class Services extends Component<RouteComponentProps & PathPrefixProps, ServicesState> {
  constructor(props: RouteComponentProps & PathPrefixProps) {
    super(props);

    this.state = {
      response: {},
      fetchServicesErrorMessage: '',
      targets: {}
    };
  }

  componentDidMount() {
    fetch(this.props.pathPrefix + '/api/v1/targets')
      .then(resp => {
        if (resp.ok) {
          return resp.json();
        } else {
          throw new Error('Unexpected response status when fetching services: ' + resp.statusText); // TODO extract error
        }
      })
      .then(json => {
        this.setState({ response: json, targets: this.processResponse(json) });
        console.warn(this.state.response)
      })
      .catch(err => {
        this.setState({ fetchServicesErrorMessage: err });
      });
  }

  isHealthy = (health: string) => {
    return health === 'up';
  }

  processResponse = (resp: any) => {
    let activeTargets = resp.data.activeTargets,
      targets: any = {};

    // Get total targets of each type
    for (const target of activeTargets) {
      let name = target.scrapePool;
      if (targets[name] === undefined) {
        targets[name] = {
          total: 1,
          active: 0
        };
        if (target.health === 'up') {
          targets[name].active = 1;
        }
      } else {
        targets[name].total++;
        if (this.isHealthy(target.health)) {
          targets[name].active++;
        }
      }
    }
    return targets;
  }

  render() {
    return (
      <>
       <h2>Service Discovery</h2>
       <ul>
         {
           Object.keys(this.state.targets).map((val, i) => <li key={Object.keys(this.state.targets)[i]}> {val} ({ this.state.targets[val].active } / { this.state.targets[val].total } active targets) </li>)
         }
       </ul>
       <hr/>
     </>
    );
  }
}

export default Services;

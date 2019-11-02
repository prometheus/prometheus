import React, { FC } from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faSpinner } from '@fortawesome/free-solid-svg-icons';

const Loader: FC<{}> = () => <FontAwesomeIcon icon={faSpinner} spin />;

export default Loader;

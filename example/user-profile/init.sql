
-- core authorization, google & microsoft

CREATE TABLE public.gsi_authorized_user (
    user_id character varying NOT NULL,
    id character varying NOT NULL,
    payload jsonb NOT NULL -- todo: enable simple symetric encryption 
);


ALTER TABLE ONLY public.gsi_authorized_user
    ADD CONSTRAINT gsi_authorized_user_pkey PRIMARY KEY (user_id, id);


CREATE TABLE public.organization (
    gid SERIAL,
    namespace character varying NOT NULL,
    id character varying NOT NULL,
    data jsonb NOT NULL,
    meta jsonb NOT NULL
);

ALTER TABLE ONLY public.organization
    ADD CONSTRAINT organization_pkey PRIMARY KEY (namespace, id);

CREATE TABLE public.user_profile (
    gid SERIAL,
    namespace character varying NOT NULL,
    ref_id_1 character varying, -- refer to organization
    id character varying NOT NULL,
    data jsonb NOT NULL,
    meta jsonb NOT NULL
);

ALTER TABLE ONLY public.user_profile
    ADD CONSTRAINT user_profile_pkey PRIMARY KEY (namespace, ref_id_1, id);

CREATE TABLE public.user_profile_thumbnail (
    gid SERIAL,
    namespace character varying NOT NULL,
    ref_id_1 character varying, -- refer to organization
    ref_id_2 character varying, -- refer to user_profile
    id character varying NOT NULL,
    data jsonb NOT NULL,
    meta jsonb NOT NULL
);

ALTER TABLE ONLY public.user_profile_thumbnail
    ADD CONSTRAINT user_profile_thumbnail_pkey PRIMARY KEY (namespace, id, ref_id_1, ref_id_2);
